package export

import (
	"fmt"
	"os"

	"github.com/NVIDIA/mig-parted/cmd/util"
)

func exportPlacements(f *Flags) error {
	spec, err := exportMigPlacements()
	if err != nil {
		return err
	}
	err = WriteOutput(os.Stdout, spec, f)
	if err != nil {
		return err
	}

	return nil
}

func exportMigPlacements() (map[int]map[int]string, error) {
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
