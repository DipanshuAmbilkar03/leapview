package composectl

import deploymentqualificationbarrier "github.com/flidai/leapview/internal/deployment/qualificationbarrier"

type qualificationRecoveryReport struct {
	ReleaseFinalizationBoundary string                                                `json:"releaseFinalizationBoundary,omitempty"`
	ReleaseFinalizationEvidence *deploymentqualificationbarrier.BuildResponseEvidence `json:"releaseFinalizationEvidence,omitempty"`
	SchemaVersion               int                                                   `json:"schemaVersion"`
	Result                      string                                                `json:"result"`
	Stage                       string                                                `json:"stage"`
	Image                       string                                                `json:"image"`
	Phases                      []qualificationPhaseEvidence                          `json:"phases"`
	Assertions                  struct {
		ManagedUpload        bool `json:"managedUpload"`
		ReleaseFinalization  bool `json:"releaseFinalization"`
		DeploymentActivation bool `json:"deploymentActivation"`
		RefreshRecovery      bool `json:"refreshRecovery"`
		QueryStreamReconnect bool `json:"queryStreamReconnect"`
		BoundedDisk          bool `json:"boundedDisk"`
	} `json:"assertions"`
	BoundedState struct {
		DiskBeforeKiB        int64 `json:"diskBeforeKiB"`
		DiskAfterKiB         int64 `json:"diskAfterKiB"`
		DiskGrowthKiB        int64 `json:"diskGrowthKiB"`
		DiskGrowthLimitKiB   int64 `json:"diskGrowthLimitKiB"`
		StaleRecoveryEntries int64 `json:"staleRecoveryEntries"`
	} `json:"boundedState"`
}
