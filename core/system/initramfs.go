package system

import (
	"fmt"

	"github.com/vanilla-os/albius/core/util"
)

func UpdateInitramfs(root string) error {
	var err error

	err = MountChrootRuntime(root)
	if err != nil {
		return err
	}

	updInitramfsCmd := "update-initramfs -c -k all"
	err = util.RunInChroot(root, updInitramfsCmd)
	if err != nil {
		return fmt.Errorf("failed to run update-initramfs command: %s", err)
	}

	return UnmountChrootRuntime(root)
}
