package v1alpha1

import runtime "k8s.io/apimachinery/pkg/runtime"

func (in *AppConfig) DeepCopyInto(out *AppConfig) {
	*out = *in
}

func (in *AppConfig) DeepCopy() *AppConfig {
	if in == nil {
		return nil
	}
	out := new(AppConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *CorrelationConfig) DeepCopyInto(out *CorrelationConfig) {
	*out = *in
}

func (in *CorrelationConfig) DeepCopy() *CorrelationConfig {
	if in == nil {
		return nil
	}
	out := new(CorrelationConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *HealthCheckConfig) DeepCopyInto(out *HealthCheckConfig) {
	*out = *in
}

func (in *HealthCheckConfig) DeepCopy() *HealthCheckConfig {
	if in == nil {
		return nil
	}
	out := new(HealthCheckConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *HeartbeatMonitorConfig) DeepCopyInto(out *HeartbeatMonitorConfig) {
	*out = *in
}

func (in *HeartbeatMonitorConfig) DeepCopy() *HeartbeatMonitorConfig {
	if in == nil {
		return nil
	}
	out := new(HeartbeatMonitorConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *JobMonitorConfig) DeepCopyInto(out *JobMonitorConfig) {
	*out = *in
}

func (in *JobMonitorConfig) DeepCopy() *JobMonitorConfig {
	if in == nil {
		return nil
	}
	out := new(JobMonitorConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *KwatchConfig) DeepCopyInto(out *KwatchConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

func (in *KwatchConfig) DeepCopy() *KwatchConfig {
	if in == nil {
		return nil
	}
	out := new(KwatchConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *KwatchConfig) DeepCopyObject() runtime.Object {
	out := new(KwatchConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *KwatchConfigList) DeepCopyInto(out *KwatchConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]KwatchConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *KwatchConfigList) DeepCopy() *KwatchConfigList {
	if in == nil {
		return nil
	}
	out := new(KwatchConfigList)
	in.DeepCopyInto(out)
	return out
}

func (in *KwatchConfigList) DeepCopyObject() runtime.Object {
	out := new(KwatchConfigList)
	in.DeepCopyInto(out)
	return out
}

func (in *KwatchConfigSpec) DeepCopyInto(out *KwatchConfigSpec) {
	*out = *in
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Reasons != nil {
		in, out := &in.Reasons, &out.Reasons
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.IgnoreContainerNames != nil {
		in, out := &in.IgnoreContainerNames, &out.IgnoreContainerNames
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.IgnorePodNames != nil {
		in, out := &in.IgnorePodNames, &out.IgnorePodNames
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.IgnoreLogPatterns != nil {
		in, out := &in.IgnoreLogPatterns, &out.IgnoreLogPatterns
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.SeverityByOwnerKind != nil {
		in, out := &in.SeverityByOwnerKind, &out.SeverityByOwnerKind
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Silences != nil {
		in, out := &in.Silences, &out.Silences
		*out = make([]SilenceRule, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	out.Correlation = in.Correlation
	out.PvcMonitor = in.PvcMonitor
	out.NodeMonitor = in.NodeMonitor
	out.RolloutMonitor = in.RolloutMonitor
	out.JobMonitor = in.JobMonitor
	out.HeartbeatMonitor = in.HeartbeatMonitor
	out.HealthCheck = in.HealthCheck
	out.App = in.App
}

func (in *KwatchConfigSpec) DeepCopy() *KwatchConfigSpec {
	if in == nil {
		return nil
	}
	out := new(KwatchConfigSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *NodeMonitorConfig) DeepCopyInto(out *NodeMonitorConfig) {
	*out = *in
}

func (in *NodeMonitorConfig) DeepCopy() *NodeMonitorConfig {
	if in == nil {
		return nil
	}
	out := new(NodeMonitorConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *PvcMonitorConfig) DeepCopyInto(out *PvcMonitorConfig) {
	*out = *in
}

func (in *PvcMonitorConfig) DeepCopy() *PvcMonitorConfig {
	if in == nil {
		return nil
	}
	out := new(PvcMonitorConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *RolloutMonitorConfig) DeepCopyInto(out *RolloutMonitorConfig) {
	*out = *in
}

func (in *RolloutMonitorConfig) DeepCopy() *RolloutMonitorConfig {
	if in == nil {
		return nil
	}
	out := new(RolloutMonitorConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *SilenceRule) DeepCopyInto(out *SilenceRule) {
	*out = *in
	if in.Namespaces != nil {
		in, out := &in.Namespaces, &out.Namespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Reasons != nil {
		in, out := &in.Reasons, &out.Reasons
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.PodNamePatterns != nil {
		in, out := &in.PodNamePatterns, &out.PodNamePatterns
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *SilenceRule) DeepCopy() *SilenceRule {
	if in == nil {
		return nil
	}
	out := new(SilenceRule)
	in.DeepCopyInto(out)
	return out
}
