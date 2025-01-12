package albius

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vanilla-os/prometheus"
)

func Unsquashfs(filesystem, destination string, force bool) error {
	unsquashfsCmd := "unsquashfs%s -d %s %s"

	var forceFlag string
	if force {
		forceFlag = " -f"
	} else {
		forceFlag = ""
	}

	err := RunCommand(fmt.Sprintf(unsquashfsCmd, forceFlag, destination, filesystem))
	if err != nil {
		return fmt.Errorf("failed to run unsquashfs: %s", err)
	}

	return nil
}

func MakeFs(part *Partition) error {
	var err error
	switch part.Filesystem {
	case FAT16:
		makefsCmd := "mkfs.fat -I -F 16 %s"
		err = RunCommand(fmt.Sprintf(makefsCmd, part.Path))
	case FAT32:
		makefsCmd := "mkfs.fat -I -F 32 %s"
		err = RunCommand(fmt.Sprintf(makefsCmd, part.Path))
	case EXT2, EXT3, EXT4:
		makefsCmd := "mkfs.%s -F %s"
		err = RunCommand(fmt.Sprintf(makefsCmd, part.Filesystem, part.Path))
	case LINUX_SWAP:
		makefsCmd := "mkswap -f %s"
		err = RunCommand(fmt.Sprintf(makefsCmd, part.Path))
	case HFS, HFS_PLUS, UDF:
		return fmt.Errorf("unsupported filesystem: %s", part.Filesystem)
	default:
		makefsCmd := "mkfs.%s -f %s"
		err = RunCommand(fmt.Sprintf(makefsCmd, part.Filesystem, part.Path))
	}

	if err != nil {
		return fmt.Errorf("failed to make %s filesystem for %s: %s", part.Filesystem, part.Path, err)
	}

	return nil
}

func GenFstab(targetRoot string, entries [][]string) error {
	fstabHeader := `# /etc/fstab: static file system information.
#
# Use 'blkid' to print the universally unique identifier for a
# device; this may be used with UUID= as a more robust way to name devices
# that works even if disks are added and removed. See fstab(5).
#
# <file system>  <mount point>  <type>  <options>  <dump>  <pass>`

	file, err := os.Create(fmt.Sprintf("%s/etc/fstab", targetRoot))
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = file.Write(append([]byte(fstabHeader), '\n'))
	if err != nil {
		return err
	}

	for _, entry := range entries {
		fmtEntry := strings.Join(entry, " ")
		_, err = file.Write(append([]byte(fmtEntry), '\n'))
		if err != nil {
			return err
		}
	}

	return nil
}

func UpdateInitramfs(root string) error {
	// Setup mountpoints
	mountOrder := []string{"/dev", "/dev/pts", "/proc", "/sys"}
	for _, mount := range mountOrder {
		if err := RunCommand(fmt.Sprintf("mount --bind %s %s%s", mount, root, mount)); err != nil {
			return fmt.Errorf("error mounting %s to chroot: %s", mount, err)
		}
	}

	updInitramfsCmd := "update-initramfs -c -k all"
	err := RunInChroot(root, updInitramfsCmd)
	if err != nil {
		return fmt.Errorf("failed to run update-initramfs command: %s", err)
	}

	// Cleanup mountpoints
	unmountOrder := []string{"/dev/pts", "/dev", "/proc", "/sys"}
	for _, mount := range unmountOrder {
		if err := RunCommand(fmt.Sprintf("umount %s%s", root, mount)); err != nil {
			return fmt.Errorf("error unmounting %s fron chroot: %s", mount, err)
		}
	}

	return nil
}

func OCISetup(imageSource, storagePath, destination string, verbose bool) error {
	pmt, err := prometheus.NewPrometheus(filepath.Join(storagePath, "storage"), "overlay", 0)
	if err != nil {
		return fmt.Errorf("failed to create Prometheus instance: %s", err)
	}

	// Create tmp directory in root's /var to store podman's temp files, since /var/tmp in
	// the ISO is tied to the user's RAM and can run out of space pretty quickly
	storageTmpDir := filepath.Join(storagePath, "tmp")
	err = os.Mkdir(storageTmpDir, 0644)
	if err != nil {
		return fmt.Errorf("failed to create storage tmp dir: %s", err)
	}
	err = RunCommand(fmt.Sprintf("mount --bind %s %s", storageTmpDir, "/var/tmp"))
	if err != nil {
		return fmt.Errorf("failed to mount bind storage tmp dir: %s", err)
	}

	storedImageName := strings.ReplaceAll(imageSource, "/", "-")
	manifest, err := pmt.PullImage(imageSource, storedImageName)
	if err != nil {
		return fmt.Errorf("failed to pull OCI image: %s", err)
	}

	fmt.Printf("Image pulled with digest %s\n", manifest.Config.Digest)

	image, err := pmt.GetImageByDigest(manifest.Config.Digest)
	if err != nil {
		return fmt.Errorf("failed to get image from digest: %s", err)
	}

	mountPoint, err := pmt.MountImage(image.TopLayer)
	if err != nil {
		return fmt.Errorf("failed to mount image at %s: %s", image.TopLayer, err)
	}

	fmt.Printf("Image mounted at %s\n", mountPoint)

	// Rsync image into destination
	fmt.Printf("Copying image to %s\n", destination)

	var verboseFlag string
	if verbose {
		verboseFlag = "v"
	} else {
		verboseFlag = ""
	}
	err = RunCommand(fmt.Sprintf("rsync -a%sxHAX --numeric-ids %s/ %s/", verboseFlag, mountPoint, destination))
	if err != nil {
		return fmt.Errorf("failed to sync image contents to %s: %s", destination, err)
	}

	// Remove storage from destination
	err = RunCommand(fmt.Sprintf("umount -l %s/storage/graph/overlay", storagePath))
	if err != nil {
		return fmt.Errorf("failed to unmount image: %s", err)
	}

	// Unmount tmp storage directory
	err = RunCommand("umount -l /var/tmp")
	if err != nil {
		return fmt.Errorf("failed to unmount storage tmp dir: %s", err)
	}
	entries, err := os.ReadDir(storageTmpDir)
	if err != nil {
		return fmt.Errorf("failed to read from storage tmp dir: %s", err)
	}
	for _, entry := range entries {
		err = os.RemoveAll(filepath.Join(storageTmpDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to remove %s from storage tmp dir: %s", entry.Name(), err)
		}
	}

	// Store the digest in destination as it may be used by the update manager
	err = os.WriteFile(filepath.Join(destination, ".oci_digest"), []byte(manifest.Config.Digest), 0644)
	if err != nil {
		return fmt.Errorf("failed to save digest in %s: %s", destination, err)
	}

	return nil
}
