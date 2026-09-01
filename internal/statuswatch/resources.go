package statuswatch

import "k8s.io/apimachinery/pkg/runtime/schema"

var (
	apiServiceGVR = schema.GroupVersionResource{
		Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices",
	}
	crdGVR = schema.GroupVersionResource{
		Group: "apiextensions.k8s.io", Version: "v1",
		Resource: "customresourcedefinitions",
	}
	validatingAdmissionPolicyGVR = schema.GroupVersionResource{
		Group: "admissionregistration.k8s.io", Version: "v1",
		Resource: "validatingadmissionpolicies",
	}
	validatingAdmissionBindingGVR = schema.GroupVersionResource{
		Group: "admissionregistration.k8s.io", Version: "v1",
		Resource: "validatingadmissionpolicybindings",
	}
	mutatingAdmissionPolicyGVR = schema.GroupVersionResource{
		Group: "admissionregistration.k8s.io", Version: "v1",
		Resource: "mutatingadmissionpolicies",
	}
	mutatingAdmissionBindingGVR = schema.GroupVersionResource{
		Group: "admissionregistration.k8s.io", Version: "v1",
		Resource: "mutatingadmissionpolicybindings",
	}
	certificateRequestGVR = schema.GroupVersionResource{
		Group: "certificates.k8s.io", Version: "v1",
		Resource: "certificatesigningrequests",
	}
	podCertificateRequestGVR = schema.GroupVersionResource{
		Group: "certificates.k8s.io", Version: "v1",
		Resource: "podcertificaterequests",
	}
	flowSchemaGVR = schema.GroupVersionResource{
		Group: "flowcontrol.apiserver.k8s.io", Version: "v1",
		Resource: "flowschemas",
	}
	priorityLevelGVR = schema.GroupVersionResource{
		Group: "flowcontrol.apiserver.k8s.io", Version: "v1",
		Resource: "prioritylevelconfigurations",
	}
	legacyEndpointsGVR = schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "endpoints",
	}
)

type staticWatch struct {
	gvr      schema.GroupVersionResource
	resource string
	rules    map[string]map[string]bool
}

var staticStatusWatches = []staticWatch{
	{
		gvr:      mutatingAdmissionPolicyGVR,
		resource: "mutatingadmissionpolicy",
		rules:    defaultConditionRulesWithAdmission(),
	},
	{
		gvr:      mutatingAdmissionBindingGVR,
		resource: "mutatingadmissionpolicybinding",
		rules:    defaultConditionRulesWithAdmission(),
	},
	{
		gvr:      certificateRequestGVR,
		resource: "certificatesigningrequest",
		rules: map[string]map[string]bool{
			"Denied": {"True": true},
			"Failed": {"True": true},
		},
	},
	{
		gvr:      podCertificateRequestGVR,
		resource: "podcertificaterequest",
		rules: map[string]map[string]bool{
			"Denied": {"True": true},
			"Failed": {"True": true},
		},
	},
	{
		gvr:      flowSchemaGVR,
		resource: "flowschema",
		rules: map[string]map[string]bool{
			"Dangling": {"True": true},
			"Invalid":  {"True": true},
		},
	},
	{
		gvr:      priorityLevelGVR,
		resource: "prioritylevelconfiguration",
		rules: map[string]map[string]bool{
			"Dangling": {"True": true},
			"Invalid":  {"True": true},
		},
	},
	{gvr: legacyEndpointsGVR, resource: "endpoints"},
}
