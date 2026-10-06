package kube

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/kubeclient"
)

// Event reasons the cluster autoscaler and Karpenter use, in Normal
// events, to say what they decided for a pod. They are kept as notes
// beside the Warning events: the scheduling explainer quotes them.
const (
	ReasonTriggeredScaleUp  = "TriggeredScaleUp"
	ReasonNotTriggerScaleUp = "NotTriggerScaleUp"
	ReasonNominated         = "Nominated"
)

// autoscalerNormalReasons are the Normal event reasons EventNote keeps.
var autoscalerNormalReasons = []string{
	ReasonTriggeredScaleUp, ReasonNotTriggerScaleUp, ReasonNominated,
}

func autoscalerReason(reason string) bool {
	for _, r := range autoscalerNormalReasons {
		if r == reason {
			return true
		}
	}
	return false
}

// scaleEventFactories makes one Event informer per autoscaler reason.
// The API server cannot select several reasons at once, and a factory
// holds one Event informer, so each reason gets its own factory; the
// volume is a few events per pending pod.
func scaleEventFactories(
	cfg SourceConfig, handler cache.ResourceEventHandler,
) ([]informers.SharedInformerFactory, error) {
	var out []informers.SharedInformerFactory
	for _, reason := range autoscalerNormalReasons {
		selector := fields.OneTermEqualSelector("reason", reason).String()
		factory := informers.NewSharedInformerFactoryWithOptions(
			kubernetes.Interface(cfg.Client), cfg.Resync,
			informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = selector
			}))
		_, err := factory.Core().V1().Events().Informer().
			AddEventHandler(kubeclient.SafeEventHandler(
				"inventory", "autoscaler-events", handler))
		if err != nil {
			return nil, err
		}
		out = append(out, factory)
	}
	return out, nil
}
