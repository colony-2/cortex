package temporal

import (
    "time"

    "go.temporal.io/server/common/dynamicconfig"
)

// DisableToggles captures which background subsystems to disable.
type DisableToggles struct {
    Scanners          bool
    ParentClosePolicy bool
    Nexus             bool
}

// DefaultDynamicConfigForEmbedded returns a dynamic config client configured
// based on the requested toggles to reduce background activity that can cause
// noisy shutdowns. Leave all toggles false for a complete cluster.
func DefaultDynamicConfigForEmbedded(t DisableToggles) dynamicconfig.Client {
    mc := dynamicconfig.NewMemoryClient()

    if t.Scanners {
        mc.OverrideSetting(dynamicconfig.TaskQueueScannerEnabled, false)
        mc.OverrideSetting(dynamicconfig.HistoryScannerEnabled, false)
        mc.OverrideSetting(dynamicconfig.ExecutionsScannerEnabled, false)
        mc.OverrideSetting(dynamicconfig.BuildIdScavengerEnabled, false)
    }

    if t.ParentClosePolicy {
        mc.OverrideSetting(dynamicconfig.EnableParentClosePolicyWorker, false)
    }

    if t.Nexus {
        mc.OverrideSetting(dynamicconfig.EnableNexus, false)
        // Also shorten any residual long-poll timeouts to speed shutdown if toggled.
        mc.OverrideSetting(dynamicconfig.RefreshNexusEndpointsLongPollTimeout, 1*time.Second)
    }

    return mc
}
