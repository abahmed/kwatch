package controller

type ReplicaSetProcessor interface {
	ProcessReplicaSet(string, bool) error
}

type JobProcessor interface {
	ProcessJob(string, bool) error
}

type DeploymentProcessor interface {
	ProcessDeployment(string, bool) error
}

type DaemonSetProcessor interface {
	ProcessDaemonSet(string, bool) error
}

type StatefulSetProcessor interface {
	ProcessStatefulSet(string, bool) error
}

type CronJobProcessor interface {
	ProcessCronJob(string, bool) error
}

type HPAProcessor interface {
	ProcessHorizontalPodAutoscaler(string, bool) error
}

type PDBProcessor interface {
	ProcessPdb(string, bool) error
}
