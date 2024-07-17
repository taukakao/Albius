package system

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/vanilla-os/albius/core/util"
)

var requiredBinds = []string{"/dev", "/dev/pts", "/proc", "/sys", "/run"}

func MountChrootRuntime(targetRoot string) error {
	if targetRoot == "" {
		return errors.New("cannot mount chroot, no target root specified")
	}

	for _, bind := range requiredBinds {
		targetBind := filepath.Join(targetRoot, bind)
		err := util.RunCommand(fmt.Sprintf("mount --bind %s %s", bind, targetBind))
		if err != nil {
			return fmt.Errorf("failed to mount %s to %s: %s", bind, targetRoot, err)
		}
	}

	return nil
}

func UnmountChrootRuntime(targetRoot string) error {
	if targetRoot == "" {
		return errors.New("cannot unmount chroot, no target root specified")
	}

	requiredBindsReverse := make([]string, len(requiredBinds))
	copy(requiredBindsReverse, requiredBinds)
	slices.Reverse[[]string](requiredBindsReverse)

	for _, bind := range requiredBindsReverse {
		targetBind := filepath.Join(targetRoot, bind)
		err := util.RunCommand(fmt.Sprintf("umount --recursive %s", targetBind))
		if err != nil {
			return fmt.Errorf("failed to umount %s: %s", targetRoot, err)
		}
	}

	return nil
}
