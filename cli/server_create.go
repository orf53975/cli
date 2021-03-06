package cli

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/crypto/ssh"
)

func newServerCreateCommand(cli *CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "create FLAGS",
		Short:                 "Create a server",
		Args:                  cobra.NoArgs,
		TraverseChildren:      true,
		DisableFlagsInUseLine: true,
		PreRunE:               cli.ensureToken,
		RunE:                  cli.wrap(runServerCreate),
	}
	cmd.Flags().String("name", "", "Server name")
	cmd.MarkFlagRequired("name")

	cmd.Flags().String("type", "", "Server type (id or name)")
	cmd.Flag("type").Annotations = map[string][]string{
		cobra.BashCompCustom: {"__hcloud_servertype_names"},
	}
	cmd.MarkFlagRequired("type")

	cmd.Flags().String("image", "", "Image (id or name)")
	cmd.Flag("image").Annotations = map[string][]string{
		cobra.BashCompCustom: {"__hcloud_image_names"},
	}
	cmd.MarkFlagRequired("image")

	cmd.Flags().String("location", "", "Location (ID or name)")
	cmd.Flag("location").Annotations = map[string][]string{
		cobra.BashCompCustom: {"__hcloud_location_names"},
	}

	cmd.Flags().String("datacenter", "", "Datacenter (ID or name)")
	cmd.Flag("datacenter").Annotations = map[string][]string{
		cobra.BashCompCustom: {"__hcloud_datacenter_names"},
	}

	cmd.Flags().StringSlice("ssh-key", nil, "ID or name of SSH key to inject (can be specified multiple times)")
	cmd.Flag("ssh-key").Annotations = map[string][]string{
		cobra.BashCompCustom: {"__hcloud_sshkey_names"},
	}

	cmd.Flags().String("user-data-from-file", "", "Read user data from specified file (use - to read from stdin)")

	cmd.Flags().Bool("start-after-create", true, "Start server right after creation (default: true)")

	return cmd
}

func runServerCreate(cli *CLI, cmd *cobra.Command, args []string) error {
	opts, err := optsFromFlags(cli, cmd.Flags())
	if err != nil {
		return err
	}

	result, _, err := cli.Client().Server.Create(cli.Context, opts)
	if err != nil {
		return err
	}

	if err := cli.ActionProgress(cli.Context, result.Action); err != nil {
		return err
	}
	if err := cli.WaitForActions(cli.Context, result.NextActions); err != nil {
		return err
	}

	fmt.Printf("Server %d created\n", result.Server.ID)
	fmt.Printf("IPv4: %s\n", result.Server.PublicNet.IPv4.IP.String())

	// Only print the root password if it's not empty,
	// which is only the case if it wasn't created with an SSH key.
	if result.RootPassword != "" {
		fmt.Printf("Root password: %s\n", result.RootPassword)
	}

	return nil
}

func optsFromFlags(cli *CLI, flags *pflag.FlagSet) (opts hcloud.ServerCreateOpts, err error) {
	name, _ := flags.GetString("name")
	serverType, _ := flags.GetString("type")
	image, _ := flags.GetString("image")
	location, _ := flags.GetString("location")
	datacenter, _ := flags.GetString("datacenter")
	userDataFile, _ := flags.GetString("user-data-from-file")
	startAfterCreate, _ := flags.GetBool("start-after-create")
	sshKeys, _ := flags.GetStringSlice("ssh-key")

	opts = hcloud.ServerCreateOpts{
		Name: name,
		ServerType: &hcloud.ServerType{
			Name: serverType,
		},
		Image: &hcloud.Image{
			Name: image,
		},
		StartAfterCreate: &startAfterCreate,
	}

	if userDataFile != "" {
		var data []byte
		if userDataFile == "-" {
			data, err = ioutil.ReadAll(os.Stdin)
		} else {
			data, err = ioutil.ReadFile(userDataFile)
		}
		if err != nil {
			return
		}
		opts.UserData = string(data)
	}

	for _, sshKeyIDOrName := range sshKeys {
		var sshKey *hcloud.SSHKey
		sshKey, _, err = cli.Client().SSHKey.Get(cli.Context, sshKeyIDOrName)
		if err != nil {
			return
		}

		if sshKey == nil {
			sshKey, err = getSSHKeyForFingerprint(cli, sshKeyIDOrName)
			if err != nil {
				return
			}
		}

		if sshKey == nil {
			err = fmt.Errorf("SSH key not found: %s", sshKeyIDOrName)
			return
		}
		opts.SSHKeys = append(opts.SSHKeys, sshKey)
	}
	if datacenter != "" {
		opts.Datacenter = &hcloud.Datacenter{Name: datacenter}
	}
	if location != "" {
		opts.Location = &hcloud.Location{Name: location}
	}

	return
}

func getSSHKeyForFingerprint(cli *CLI, file string) (sshKey *hcloud.SSHKey, err error) {
	var (
		fileContent []byte
		publicKey   ssh.PublicKey
	)

	if fileContent, err = ioutil.ReadFile(file); err == os.ErrNotExist {
		err = nil
		return
	} else if err != nil {
		err = fmt.Errorf("lookup SSH key by fingerprint: %v", err)
		return
	}

	if publicKey, _, _, _, err = ssh.ParseAuthorizedKey(fileContent); err != nil {
		err = fmt.Errorf("lookup SSH key by fingerprint: %v", err)
		return
	}
	sshKey, _, err = cli.Client().SSHKey.GetByFingerprint(cli.Context, ssh.FingerprintLegacyMD5(publicKey))
	if err != nil {
		err = fmt.Errorf("lookup SSH key by fingerprint: %v", err)
		return
	}
	if sshKey == nil {
		err = fmt.Errorf("SSH key not found by using fingerprint of public key: %s", file)
		return
	}
	return
}
