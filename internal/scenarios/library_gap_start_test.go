package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// gapStartScenarios are releases whose new pods cannot start because of
// a hook, an image reference, a pull or a profile.
func gapStartScenarios() []scenario {
	return []scenario{
		gapPostStartHookFails(), gapInvalidImageName(), gapSlowImagePull(),
		gapSeccompProfileMissing(), gapPullSecretWrongType(),
	}
}

// gapWaiting turns the container's waiting state into reason and message.
func gapWaiting(reason, message string) podState {
	return func(_ *cluster, pod *corev1.Pod) {
		waiting := pod.Status.ContainerStatuses[0].State.Waiting
		waiting.Reason, waiting.Message = reason, message
	}
}

// gapPostStartHookFails: a release adds a postStart hook that exits 1;
// the kubelet kills the container each time and the pod restarts in a
// loop while the application's own log is clean.
func gapPostStartHookFails() scenario {
	return scenario{
		expect: expectation{
			Name: "release-post-start-hook-fails",
			Description: "A release adds a postStart hook that exits 1; " +
				"the kubelet kills the container after each start " +
				"(FailedPostStartHook) and the pod restarts in a loop.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.Containers[0].Lifecycle = &corev1.Lifecycle{
					PostStart: &corev1.LifecycleHandler{
						Exec: &corev1.ExecAction{Command: []string{
							"/bin/sh", "-c", "/app/warmup.sh"}},
					},
				}
			})
			hook := "command '/bin/sh -c /app/warmup.sh' exited with " +
				"1: , message: \"warmup: cache host unreachable\\n\""
			gapRounds(c, w, 6, 45*time.Second, func(round int32) podState {
				return func(c *cluster, pod *corev1.Pod) {
					crashLoop(137, "Error", "", round)(c, pod)
					gapWaiting("PostStartHookError", hook)(c, pod)
				}
			}, func(pod *corev1.Pod, round int32) {
				gapKubelet(c, pod, "FailedPostStartHook", "Exec "+
					"lifecycle hook ([/bin/sh -c /app/warmup.sh]) for "+
					"Container \"app\" in Pod \""+pod.Name+"_"+
					pod.Namespace+"\" failed - error: "+hook, round)
			})
		},
	}
}

// gapInvalidImageName: a release writes the image as
// registry.example.com/Shop/API:2.6; repository names must be lowercase,
// so the kubelet cannot even parse it. It is not a registry fault.
func gapInvalidImageName() scenario {
	return scenario{
		expect: expectation{
			Name: "release-invalid-image-name",
			Description: "A release sets an image reference with upper " +
				"case repository names; the new pod is stuck in " +
				"InvalidImageName.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"registry//registry.example.com",
				"node//n1"},
		},
		build: func(c *cluster) {
			image := "registry.example.com/Shop/API:2.6"
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = image
			})
			message := "Failed to apply default image tag \"" + image +
				"\": couldn't parse image name \"" + image + "\": " +
				"invalid reference format: repository name must be " +
				"lowercase"
			gapRounds(c, w, 6, 45*time.Second, func(int32) podState {
				return gapPending(waiting("InvalidImageName", message))
			}, func(pod *corev1.Pod, round int32) {
				gapKubelet(c, pod, "InspectFailed", message, round)
				gapKubelet(c, pod, "Failed", "Error: InvalidImageName",
					round)
			})
		},
	}
}

// gapSlowImagePull: a release uses a 4 GB image; on a node that does not
// have it the pull outlasts the kubelet's deadline, and the pod goes
// from ContainerCreating to ErrImagePull. The registry answers fine.
func gapSlowImagePull() scenario {
	return scenario{
		expect: expectation{
			Name: "release-image-pull-deadline",
			Description: "A release's very large image takes more than " +
				"eight minutes to pull; the pull fails with " +
				"DeadlineExceeded and the pod goes to ImagePullBackOff.",
			Root: "deployment/shop/etl", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"registry//registry.example.com",
				"node//n1"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			image := "registry.example.com/shop/etl:9"
			w := gapRelease(c, "etl", func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = image
			})
			created := c.now
			c.create(w.pod(0, "n1", createdAt(created),
				gapPending(waiting("ContainerCreating", ""))))
			c.after(8 * time.Minute)
			message := "Failed to pull image \"" + image + "\": rpc " +
				"error: code = DeadlineExceeded desc = context deadline " +
				"exceeded"
			for round := int32(1); round <= 4; round++ {
				reason := "ErrImagePull"
				if round%2 == 0 {
					reason = "ImagePullBackOff"
				}
				pod := w.pod(0, "n1", createdAt(created),
					gapPending(waiting(reason, message)))
				c.update(pod)
				gapKubelet(c, pod, "Failed", message, round)
				c.after(time.Minute)
			}
		},
	}
}

// gapSeccompProfileMissing: a release asks for a Localhost seccomp
// profile that was never installed on the node, so the runtime cannot
// create the container.
func gapSeccompProfileMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "release-seccomp-profile-missing",
			Description: "A release requests a Localhost seccomp profile " +
				"that is absent from the node; the new pod fails with " +
				"CreateContainerError \"cannot load seccomp profile\".",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			profile := "profiles/audit.json"
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.SecurityContext = &corev1.PodSecurityContext{
					SeccompProfile: &corev1.SeccompProfile{
						Type:             corev1.SeccompProfileTypeLocalhost,
						LocalhostProfile: &profile,
					},
				}
			})
			message := "failed to create containerd task: failed to " +
				"generate seccomp spec opts: cannot load seccomp " +
				"profile \"/var/lib/kubelet/seccomp/profiles/audit." +
				"json\": open /var/lib/kubelet/seccomp/profiles/audit." +
				"json: no such file or directory"
			gapRounds(c, w, 6, 45*time.Second, func(int32) podState {
				return gapPending(waiting("CreateContainerError", message))
			}, func(pod *corev1.Pod, round int32) {
				gapKubelet(c, pod, "Failed", "Error: "+message, round)
			})
		},
	}
}

// gapPullSecretWrongType: a release names the Opaque Secret regcred as
// its image pull secret; the kubelet cannot read credentials from it and
// the pull is refused as anonymous. The Secret is the cause.
func gapPullSecretWrongType() scenario {
	return scenario{
		expect: expectation{
			Name: "release-pull-secret-wrong-type",
			Description: "A release uses an Opaque Secret as its image " +
				"pull secret; the registry refuses the anonymous pull " +
				"with 401 Unauthorized.",
			Root: "secret/shop/regcred", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"registry//registry.example.com",
				"deployment/shop/api"},
		},
		build: func(c *cluster) {
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.ImagePullSecrets = []corev1.LocalObjectReference{{
					Name: c.n("regcred")}}
			})
			c.list(configSecret(c, "shop", "regcred", "token"))
			image := "registry.example.com/shop/api:2.5"
			message := "Failed to pull image \"" + image + "\": failed " +
				"to pull and unpack image \"" + image + "\": failed to " +
				"resolve reference \"" + image + "\": failed to " +
				"authorize: failed to fetch anonymous token: " +
				"unexpected status from GET request to " +
				"https://registry.example.com/token: 401 Unauthorized"
			gapRounds(c, w, 6, 45*time.Second, func(round int32) podState {
				reason := "ErrImagePull"
				if round%2 == 0 {
					reason = "ImagePullBackOff"
				}
				return gapPending(waiting(reason, message))
			}, func(pod *corev1.Pod, round int32) {
				gapKubelet(c, pod, "Failed", message, round)
			})
		},
	}
}
