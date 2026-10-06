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

func (in *MaintenanceConfig) DeepCopyInto(out *MaintenanceConfig) {
	*out = *in
}

func (in *MaintenanceConfig) DeepCopy() *MaintenanceConfig {
	if in == nil {
		return nil
	}
	out := new(MaintenanceConfig)
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
	out.Maintenance = in.Maintenance
	out.Telemetry = in.Telemetry
	out.HeartbeatMonitor = in.HeartbeatMonitor
	out.App = in.App
}

func deepCopySpecMaps(in, out *KwatchConfigSpec) {
	if in.Upgrader != nil {
		out.Upgrader = runtime.DeepCopyJSONValue(in.Upgrader).(map[string]interface{})
	}
	out.ActiveProbeMonitor = deepCopyMonitorConfig(in.ActiveProbeMonitor)
	out.Crd = deepCopyMonitorConfig(in.Crd)
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

func deepCopyMonitorConfig(v MonitorConfig) MonitorConfig {
	if v == nil {
		return nil
	}
	return runtime.DeepCopyJSONValue(
		map[string]interface{}(v),
	).(map[string]interface{})
}

func (in *KwatchConfigSpec) DeepCopy() *KwatchConfigSpec {
	if in == nil {
		return nil
	}
	out := new(KwatchConfigSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *TelemetryConfig) DeepCopyInto(out *TelemetryConfig) {
	*out = *in
}

func (in *TelemetryConfig) DeepCopy() *TelemetryConfig {
	if in == nil {
		return nil
	}
	out := new(TelemetryConfig)
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
	if in.ContainerMessages != nil {
		in, out := &in.ContainerMessages, &out.ContainerMessages
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.EventMessages != nil {
		in, out := &in.EventMessages, &out.EventMessages
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
