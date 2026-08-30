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
	if in.Escalation != nil {
		out.Escalation = runtime.DeepCopyJSONValue(map[string]interface{}(in.Escalation)).(map[string]interface{})
	}
	if in.Renotify != nil {
		out.Renotify = runtime.DeepCopyJSONValue(map[string]interface{}(in.Renotify)).(map[string]interface{})
	}
}

func (in *CorrelationConfig) DeepCopy() *CorrelationConfig {
	if in == nil {
		return nil
	}
	out := new(CorrelationConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *CronJobMonitorConfig) DeepCopyInto(out *CronJobMonitorConfig) {
	*out = *in
}

func (in *CronJobMonitorConfig) DeepCopy() *CronJobMonitorConfig {
	if in == nil {
		return nil
	}
	out := new(CronJobMonitorConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *DaemonSetMonitorConfig) DeepCopyInto(out *DaemonSetMonitorConfig) {
	*out = *in
}

func (in *DaemonSetMonitorConfig) DeepCopy() *DaemonSetMonitorConfig {
	if in == nil {
		return nil
	}
	out := new(DaemonSetMonitorConfig)
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
	if in.IgnoreContainerMessages != nil {
		in, out := &in.IgnoreContainerMessages, &out.IgnoreContainerMessages
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.IgnoreNodeReasons != nil {
		in, out := &in.IgnoreNodeReasons, &out.IgnoreNodeReasons
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.IgnoreNodeMessages != nil {
		in, out := &in.IgnoreNodeMessages, &out.IgnoreNodeMessages
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.IgnoreDisruptionTerminations != nil {
		in, out := &in.IgnoreDisruptionTerminations, &out.IgnoreDisruptionTerminations
		*out = new(bool)
		**out = **in
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
	if in.SeverityByReason != nil {
		in, out := &in.SeverityByReason, &out.SeverityByReason
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	deepCopySpecMaps(in, out)
	out.Correlation = in.Correlation
	out.PvcMonitor = in.PvcMonitor
	out.NodeMonitor = in.NodeMonitor
	out.RolloutMonitor = in.RolloutMonitor
	out.DaemonSetMonitor = in.DaemonSetMonitor
	out.JobMonitor = in.JobMonitor
	out.CronJobMonitor = in.CronJobMonitor
	out.HeartbeatMonitor = in.HeartbeatMonitor
	out.HealthCheck = in.HealthCheck
	out.App = in.App
}

func deepCopySpecMaps(in, out *KwatchConfigSpec) {
	if in.Upgrader != nil {
		out.Upgrader = runtime.DeepCopyJSONValue(in.Upgrader).(map[string]interface{})
	}
	if in.Alert != nil {
		out.Alert = runtime.DeepCopyJSONValue(in.Alert).(map[string]interface{})
	}
	copyMonitor := func(v MonitorConfig) MonitorConfig {
		if v == nil {
			return nil
		}
		return runtime.DeepCopyJSONValue(map[string]interface{}(v)).(map[string]interface{})
	}
	out.ScheduleMonitor = copyMonitor(in.ScheduleMonitor)
	out.OomMonitor = copyMonitor(in.OomMonitor)
	out.PendingPodMonitor = copyMonitor(in.PendingPodMonitor)
	out.NotReadyMonitor = copyMonitor(in.NotReadyMonitor)
	out.StatefulSetMonitor = copyMonitor(in.StatefulSetMonitor)
	out.PdbMonitor = copyMonitor(in.PdbMonitor)
	out.NodeResourceMonitor = copyMonitor(in.NodeResourceMonitor)
	out.ClusterAutoscalerMonitor = copyMonitor(in.ClusterAutoscalerMonitor)
	out.HpaMonitor = copyMonitor(in.HpaMonitor)
	out.TlsMonitor = copyMonitor(in.TlsMonitor)
	out.ServiceMonitor = copyMonitor(in.ServiceMonitor)
	out.AdmissionWebhookMonitor = copyMonitor(in.AdmissionWebhookMonitor)
	out.ControlPlaneMonitor = copyMonitor(in.ControlPlaneMonitor)
	out.IngressMonitor = copyMonitor(in.IngressMonitor)
	out.NetworkPolicyMonitor = copyMonitor(in.NetworkPolicyMonitor)
	out.SmartGrouping = copyMonitor(in.SmartGrouping)
	out.Inhibition = copyMonitor(in.Inhibition)
	if in.Templates != nil {
		out.Templates = make(map[string]string, len(in.Templates))
		for k, v := range in.Templates {
			out.Templates[k] = v
		}
	}
	if in.Runbooks != nil {
		out.Runbooks = make(map[string]string, len(in.Runbooks))
		for k, v := range in.Runbooks {
			out.Runbooks[k] = v
		}
	}
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
	if in.ContainerNames != nil {
		in, out := &in.ContainerNames, &out.ContainerNames
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.LogPatterns != nil {
		in, out := &in.LogPatterns, &out.LogPatterns
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ContainerMessages != nil {
		in, out := &in.ContainerMessages, &out.ContainerMessages
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.NodeReasons != nil {
		in, out := &in.NodeReasons, &out.NodeReasons
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.NodeMessages != nil {
		in, out := &in.NodeMessages, &out.NodeMessages
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
