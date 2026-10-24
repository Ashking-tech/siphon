package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"siphon/internal/core"
	"siphon/internal/engine"
	"siphon/internal/media"
	"siphon/internal/platform"
)

type doctorReport struct {
	OK          bool         `json:"ok"`
	Tools       []media.Tool `json:"tools"`
	FreeBytes   int64        `json:"free_bytes"`
	Free        string       `json:"free"`
	Sparse      bool         `json:"sparse_supported"`
	SparseError string       `json:"sparse_error,omitempty"`
	Root        bool         `json:"root"`
	SELinux     string       `json:"selinux"`
	Engines     []string     `json:"engines"`
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Show tool versions, filesystem capabilities, and SELinux status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tools := media.Inspect(cmd.Context(), media.AllTools())
			wd, _ := os.Getwd()
			free, _ := platform.FreeSpace(wd)
			sparseErr := platform.SparseProbe(wd, 64<<20)

			selinux := "absent"
			if b, err := os.ReadFile("/sys/fs/selinux/enforce"); err == nil {
				switch strings.TrimSpace(string(b)) {
				case "1":
					selinux = "enforcing"
				case "0":
					selinux = "permissive"
				}
			}

			rep := doctorReport{
				OK:        true,
				Tools:     tools,
				FreeBytes: free,
				Free:      core.HumanBytes(free),
				Sparse:    sparseErr == nil,
				Root:      os.Geteuid() == 0,
				SELinux:   selinux,
				Engines:   engine.Names(),
			}
			if sparseErr != nil {
				rep.SparseError = sparseErr.Error()
			}

			if globals.JSON {
				emitJSON(rep)
				return nil
			}
			fmt.Printf("Siphon doctor\n")
			fmt.Printf("  directory     %s\n", wd)
			fmt.Printf("  free space    %s\n", rep.Free)
			fmt.Printf("  sparse files  %v\n", rep.Sparse)
			if sparseErr != nil {
				fmt.Printf("                %v\n", sparseErr)
			}
			fmt.Printf("  running root  %v\n", rep.Root)
			fmt.Printf("  SELinux       %s\n", rep.SELinux)
			fmt.Printf("  engines       %v\n", rep.Engines)
			fmt.Printf("  tools\n")
			for _, t := range rep.Tools {
				if t.Path == "" {
					fmt.Printf("    %-8s missing\n", t.Name)
					continue
				}
				fmt.Printf("    %-8s %-10s %s\n", t.Name, t.Version, t.Path)
			}
			return nil
		},
	}
}
