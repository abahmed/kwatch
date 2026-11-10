package crd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"syscall"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/config"
)

var configResource = schema.GroupResource{
	Group: gvr.Group, Resource: gvr.Resource,
}

// failingClient answers the first len(errs) Lists with errs, then serves
// an empty list. It reports how many Lists it saw.
func failingClient(errs ...error) (*dynamicfake.FakeDynamicClient, *int) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "KwatchConfigList"},
	)
	calls := new(int)
	client.PrependReactor("list", gvr.Resource,
		func(clienttesting.Action) (bool, runtime.Object, error) {
			*calls++
			if *calls <= len(errs) {
				return true, nil, errs[*calls-1]
			}
			return false, nil, nil
		})
	return client, calls
}

// recordedRetry sleeps on a fake clock that each wait advances.
func recordedRetry(budget time.Duration) (*StartupRetry, *[]time.Duration) {
	var waits []time.Duration
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	return &StartupRetry{
		Budget: budget,
		Now:    func() time.Time { return now },
		Sleep: func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			now = now.Add(d)
			return nil
		},
	}, &waits
}

func enabledConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.CrdConfig.Enabled = true
	return cfg
}

func TestStartupListRetriesAnUnavailableAPI(t *testing.T) {
	client, calls := failingClient(
		apierrors.NewServiceUnavailable("etcd leader changed"),
		fmt.Errorf("dial: %w", syscall.ECONNREFUSED),
		apierrors.NewTooManyRequests("slow down", 1),
		io.ErrUnexpectedEOF,
	)
	retry, waits := recordedRetry(0)

	spec, err := ApplyStartupConfigWithRetry(context.Background(),
		enabledConfig(), client, "default", retry)

	if err != nil || spec != nil {
		t.Fatalf("spec, err = %s, %v; want no KwatchConfig", spec, err)
	}
	if *calls != 5 {
		t.Fatalf("lists = %d, want 5", *calls)
	}
	want := []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
	}
	if fmt.Sprint(*waits) != fmt.Sprint(want) {
		t.Fatalf("waits = %v, want %v", *waits, want)
	}
}

func TestStartupListDoesNotRetryForbidden(t *testing.T) {
	client, calls := failingClient(apierrors.NewForbidden(
		configResource, "", errors.New("rbac")))
	retry, waits := recordedRetry(0)

	_, err := ApplyStartupConfigWithRetry(context.Background(),
		enabledConfig(), client, "default", retry)

	if !apierrors.IsForbidden(err) {
		t.Fatalf("err = %v, want Forbidden", err)
	}
	if *calls != 1 || len(*waits) != 0 {
		t.Fatalf("lists = %d, waits = %v; Forbidden was retried",
			*calls, *waits)
	}
}

func TestStartupListNotFoundMeansNoCRD(t *testing.T) {
	client, calls := failingClient(apierrors.NewNotFound(
		configResource, ""))
	retry, _ := recordedRetry(0)

	spec, err := ApplyStartupConfigWithRetry(context.Background(),
		enabledConfig(), client, "default", retry)

	if err != nil || spec != nil || *calls != 1 {
		t.Fatalf("spec, err, lists = %s, %v, %d", spec, err, *calls)
	}
}

func TestStartupRetryGivesUpAfterItsBudget(t *testing.T) {
	unavailable := apierrors.NewServiceUnavailable("down")
	retry, waits := recordedRetry(time.Minute)
	attempts := 0

	err := retry.Do(context.Background(), func(context.Context) error {
		attempts++
		return unavailable
	})

	if !apierrors.IsServiceUnavailable(err) {
		t.Fatalf("err = %v, want the last API error", err)
	}
	// 1+2+4+8+16 = 31s; the next 30s wait would pass the minute.
	if attempts != 6 || len(*waits) != 5 {
		t.Fatalf("attempts = %d, waits = %v", attempts, *waits)
	}
}

func TestStartupRetryCapsTheBackoff(t *testing.T) {
	retry, waits := recordedRetry(0)
	attempts := 0

	err := retry.Do(context.Background(), func(context.Context) error {
		attempts++
		if attempts > 8 {
			return nil
		}
		return apierrors.NewInternalError(errors.New("boom"))
	})

	if err != nil {
		t.Fatal(err)
	}
	if got := (*waits)[len(*waits)-1]; got != startupBackoffMax {
		t.Fatalf("last wait = %s, want %s", got, startupBackoffMax)
	}
}

func TestStartupRetryStopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	retry := &StartupRetry{OnRetry: func(int, time.Duration, error) {
		cancel()
	}}
	done := make(chan error, 1)

	go func() {
		done <- retry.Do(ctx, func(context.Context) error {
			return apierrors.NewServiceUnavailable("down")
		})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a canceled retry kept waiting")
	}
}

func TestStartupRetryBoundsEachAttempt(t *testing.T) {
	var deadline time.Time
	var ok bool

	_ = (*StartupRetry)(nil).Do(context.Background(),
		func(ctx context.Context) error {
			deadline, ok = ctx.Deadline()
			return nil
		})

	if !ok || deadline.IsZero() {
		t.Fatal("an attempt ran without a timeout")
	}
}

func TestRetryableAPIErrorClassification(t *testing.T) {
	for _, err := range []error{
		apierrors.NewServerTimeout(configResource, "list", 1),
		apierrors.NewTimeoutError("slow", 1),
		apierrors.NewGenericServerResponse(502, "list", configResource,
			"", "bad gateway", 0, true),
		fmt.Errorf("read: %w", syscall.ECONNRESET),
		context.DeadlineExceeded,
	} {
		if !IsRetryableAPIError(err) {
			t.Errorf("%v was not retryable", err)
		}
	}
	for _, err := range []error{
		nil, apierrors.NewUnauthorized("token"),
		apierrors.NewBadRequest("bad"), errors.New("other"),
	} {
		if IsRetryableAPIError(err) {
			t.Errorf("%v was retryable", err)
		}
	}
}

// A rebuild retries Unauthorized and Forbidden: an auth webhook can still
// be down as the API server comes back.
func TestStartupListRetriesAuthErrorsWhenAsked(t *testing.T) {
	client, calls := failingClient(
		apierrors.NewUnauthorized("webhook"),
		apierrors.NewForbidden(configResource, "", errors.New("webhook")),
	)
	retry, waits := recordedRetry(0)
	retry.AuthWindow = time.Minute

	_, err := ApplyStartupConfigWithRetry(context.Background(),
		enabledConfig(), client, "default", retry)

	if err != nil || *calls != 3 || len(*waits) != 2 {
		t.Fatalf("err, lists, waits = %v, %d, %v", err, *calls, *waits)
	}
}

// A NotFound is retried within NotFoundWindow, then taken as a missing
// type, which is reported apart from an empty list.
func TestStartupListRetriesNotFoundWithinItsWindow(t *testing.T) {
	missing := apierrors.NewNotFound(configResource, "")
	client, calls := failingClient(missing, missing)
	retry, _ := recordedRetry(0)
	retry.NotFoundWindow = 45 * time.Second

	_, typeMissing, err := ApplyStartupConfigChecked(context.Background(),
		enabledConfig(), client, "default", retry)
	if err != nil || typeMissing || *calls != 3 {
		t.Fatalf("err, missing, lists = %v, %v, %d", err, typeMissing,
			*calls)
	}

	errs := make([]error, 20)
	for i := range errs {
		errs[i] = missing
	}
	client, calls = failingClient(errs...)
	retry, waits := recordedRetry(0)
	retry.NotFoundWindow = 45 * time.Second
	_, typeMissing, err = ApplyStartupConfigChecked(context.Background(),
		enabledConfig(), client, "default", retry)
	var total time.Duration
	for _, w := range *waits {
		total += w
	}
	if err != nil || !typeMissing || total > retry.NotFoundWindow {
		t.Fatalf("err, missing, waited = %v, %v, %v", err, typeMissing,
			total)
	}
	if *calls != len(*waits)+1 || *calls < 2 {
		t.Fatalf("lists = %d, waits = %v", *calls, *waits)
	}
}
