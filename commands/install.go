package commands

import (
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"github.com/zyrouge/pho/core"
	"github.com/zyrouge/pho/utils"
)

var InstallCommand = cli.Command{
	Name:    "install",
	Aliases: []string{"add"},
	Usage:   "Install an application",
	Commands: []*cli.Command{
		&InstallGithubCommand,
		&InstallLocalCommand,
	},
}

type Installable struct {
	Name        string
	Id          string
	DownloadUrl string
	Size        int
}

type InstallableAppStatus int

const (
	InstallableAppFailed InstallableAppStatus = iota
	InstallableAppDownloading
	InstallableAppIntegrating
	InstallableAppInstalled
)

type InstallableApp struct {
	App    *core.AppConfig
	Source any
	Paths  *core.AppPaths
	Asset  *core.Asset

	Index      int
	Count      int
	StartedAt  int64
	Progress   int64
	PrintCycle int
	Status     InstallableAppStatus
}

func (x *InstallableApp) Write(data []byte) (n int, err error) {
	l := len(data)
	x.Progress += int64(l)
	return l, nil
}

func (x *InstallableApp) PrintStatus() {
	if x.PrintCycle > 0 {
		utils.TerminalErasePreviousLine()
	}
	x.PrintCycle++

	prefix := color.HiBlackString(fmt.Sprintf("[%d/%d]", x.Index+1, x.Count))
	suffix := color.HiBlackString(
		fmt.Sprintf("(%s)", utils.HumanizeSeconds(utils.TimeNowSeconds()-x.StartedAt)),
	)

	switch x.Status {
	case InstallableAppFailed:
		fmt.Printf(
			"%s %s %s %s %s\n",
			prefix,
			utils.LogExclamationPrefix,
			color.RedString(x.App.Name),
			x.App.Version,
			suffix,
		)

	case InstallableAppDownloading:
		fmt.Printf(
			"%s %s %s %s (%s / %s) %s\n",
			prefix,
			color.YellowString(utils.TerminalLoadingSymbol(x.PrintCycle)),
			color.CyanString(x.App.Name),
			x.App.Version,
			prettyBytes(x.Progress),
			prettyBytes(x.Asset.Size),
			suffix,
		)

	case InstallableAppIntegrating:
		fmt.Printf(
			"%s %s %s %s %s\n",
			prefix,
			color.YellowString(utils.TerminalLoadingSymbol(x.PrintCycle)),
			color.CyanString(x.App.Name),
			x.App.Version,
			suffix,
		)

	case InstallableAppInstalled:
		fmt.Printf(
			"%s %s %s %s %s\n",
			prefix,
			utils.LogTickPrefix,
			color.GreenString(x.App.Name),
			x.App.Version,
			suffix,
		)
	}
}

const printStatusTickerDuration = time.Second / 4

func (x *InstallableApp) StartStatusTicker() *time.Ticker {
	ticker := time.NewTicker(printStatusTickerDuration)
	go func() {
		for range ticker.C {
			x.PrintStatus()
		}
	}()
	return ticker
}

func InstallApps(apps []InstallableApp) (int, int) {
	success := 0
	count := len(apps)
	for i := range apps {
		x := &apps[i]
		x.Index = i
		x.Count = count
		x.StartedAt = utils.TimeNowSeconds()
		x.Status = InstallableAppDownloading
		x.PrintStatus()
		core.UpdateTransactions(func(transactions *core.Transactions) error {
			transactions.PendingInstallations[x.App.Id] = core.PendingInstallation{
				InvolvedDirs:  []string{x.Paths.Dir},
				InvolvedFiles: []string{x.Paths.Desktop},
			}
			return nil
		})
		if err := x.Install(); err != nil {
			x.Status = InstallableAppFailed
			x.PrintStatus()
			utils.LogError(err)
			break
		} else {
			x.Status = InstallableAppInstalled
			x.PrintStatus()
			success++
		}
		core.UpdateTransactions(func(transactions *core.Transactions) error {
			delete(transactions.PendingInstallations, x.App.Id)
			return nil
		})
	}
	return success, count - success
}

func (x *InstallableApp) Install() error {
	ticker := x.StartStatusTicker()
	defer ticker.Stop()
	if err := x.Download(); err != nil {
		return err
	}
	x.Status = InstallableAppIntegrating
	if err := x.Integrate(); err != nil {
		return err
	}
	if err := x.SaveConfig(); err != nil {
		return err
	}
	return nil
}

func (x *InstallableApp) Download() error {
	if err := os.MkdirAll(x.Paths.Dir, os.ModePerm); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Dir(x.Paths.Desktop), os.ModePerm); err != nil {
		return err
	}
	file, err := os.Create(x.Paths.AppImage)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := x.Asset.Download()
	if err != nil {
		return err
	}
	defer data.Close()
	mw := io.MultiWriter(file, x)
	_, err = io.Copy(mw, data)
	if err != nil {
		return err
	}
	return os.Chmod(x.Paths.AppImage, 0755)
}

func (x *InstallableApp) Integrate() error {
	tempDir := path.Join(x.Paths.Dir, "temp")
	err := os.Mkdir(tempDir, os.ModePerm)
	if err != nil {
		return err
	}
	deflated, err := core.DeflateAppImage(x.Paths.AppImage, tempDir)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	metadata, err := deflated.ExtractMetadata()
	if err != nil {
		return err
	}
	if err = metadata.CopyIconFile(x.Paths); err != nil {
		return err
	}
	if err = metadata.InstallDesktopFile(x.Paths); err != nil {
		return err
	}
	return nil
}

func (x *InstallableApp) SaveConfig() error {
	if err := core.SaveAppConfig(x.Paths.Config, x.App); err != nil {
		return err
	}
	if err := core.SaveAppSourceConfig[any](x.Paths.SourceConfig, x.Source); err != nil {
		return err
	}
	config, err := core.ReadConfig()
	if err != nil {
		return err
	}
	config.Installed[x.App.Id] = x.Paths.Dir
	return core.SaveConfig(config)
}

func prettyBytes(size int64) string {
	mb := float32(size) / 1000000
	return fmt.Sprintf("%.2f MB", mb)
}