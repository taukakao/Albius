package system

import (
	"fmt"
)

func UpdateInitramfs(root string) error {
	var err error

	err = MountChrootRuntime(root)
	if err != nil {
		return err
	}

	updInitramfsCmd := "update-initramfs -c -k all"

	fmt.Println(root, updInitramfsCmd)

	panic("finished")
	// err = util.RunInChroot(root, updInitramfsCmd)
	// if err != nil {
	// 	return fmt.Errorf("failed to run update-initramfs command: %s", err)
	// }

	// return UnmountChrootRuntime(root)
}
