package commands

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"github.com/zyrouge/pho/core"
	"github.com/zyrouge/pho/utils"
)

var SelfUpdateCommand = cli.Command{
	Name:    "self-update",
	Aliases: []string{"self-upgrade"},
	Usage:   fmt.Sprintf("Update %s", core.AppName),
	Action: func(ctx *cli.Context) error {
		release, err := core.GithubApiFetchLatestRelease(core.AppGithubOwner, core.AppGithubRepo)
		if err != nil {
			return err
		}
		arch := utils.GetSystemArch()
		var asset *core.GithubApiReleaseAsset
		for _, x := range release.Assets {
			if strings.HasSuffix(x.Name, arch) {
				asset = &x
			}
		}
		if asset == nil {
			return fmt.Errorf(
				"unable to find appropriate binary from release %s",
				release.TagName,
			)
		}
		utils.LogInfo(fmt.Sprintf("Updating to version %s...", color.CyanString(release.TagName)))
		data, err := http.Get(asset.DownloadUrl)
		if err != nil {
			return err
		}
		defer data.Body.Close()
		file, err := os.Create(os.Args[0])
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, data.Body)
		if err != nil {
			return err
		}
		utils.LogInfo(
			fmt.Sprintf(
				"%s Updated to version %s successfully!",
				utils.LogTickPrefix,
				color.CyanString(release.TagName),
			),
		)

		return nil
	},
}