package export

import (
	"fmt"

	"github.com/NVIDIA/mig-parted/cmd/util"
)

func ExportMigPlacements(c *Context) (map[int]map[int]string, error) {
	nvidiaModuleLoaded, err := util.IsNvidiaModuleLoaded()
	if err != nil {
		return nil, fmt.Errorf("error checking if nvidia module loaded: %v", err)
	}
	if !nvidiaModuleLoaded {
		return nil, fmt.Errorf("nvidia module must be loaded in order to query MIG device state")
	}

	manager := util.NewCombinedMigManager()

	return manager.GetMigPlacements()
}
