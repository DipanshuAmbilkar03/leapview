package module

import deploymentgen "github.com/flidai/leapview/internal/deployment/api/gen"

// CreateDeliveryPlanOperationID exposes the native plan command through the
// module boundary. Its transactional operation ledger owns durable replay.
const CreateDeliveryPlanOperationID = string(deploymentgen.GenOperationCreateDeliveryPlan)

// BuildDeliveryPlanOperationID exposes the native build command through the
// module boundary. Its fenced operation ledger owns replay and recovery.
const BuildDeliveryPlanOperationID = string(deploymentgen.GenOperationBuildDeliveryPlan)
