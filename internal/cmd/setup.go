package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"neite.dev/go-ship/internal/config"
	"neite.dev/go-ship/internal/docker"
)

// var imgName = "goship-app-test"

func init() {
	rootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup your servers by installing Docker and Caddy",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("setting up your servers...")

		err := docker.IsInstalled().Run()
		if err != nil {
			fmt.Println("Error running `docker --version` locally. Make sure you have docker installed.")
			return
		}

		err = docker.IsRunning().Run()
		if err != nil {
			fmt.Println("Error running `docker version` locally. Make sure docker daemon is running.")
			return
		}

		fmt.Println("reading your config file...")

		if !config.IsExists() {
			fmt.Printf("Could not find your config file. Make sure to run `goship init` first.")
			return

		}

		userCfg, err := config.ReadConfig()
		if err != nil {
			fmt.Println("Could not read your config file")
		}

		imgName := userCfg.Registry.Image
		usrName := userCfg.Registry.Username
		repoName := userCfg.Registry.Reponame

		err = docker.BuildImage(imgName, "").Run()
		if err != nil {
			fmt.Println("Error running `docker build`. Could not build your image.")
		}

		err = docker.RunContainer(3000, imgName, imgName).Run()
		if err != nil {
			fmt.Println("Error running `docker run`. Could not run container.")
		}
		err = docker.LoginToHub(usrName, userCfg.Registry.Password).Run()
		if err != nil {
			fmt.Println("error running `docker login`. Could not login to docker hub.")
			return
		}

		err = docker.RenameImage(imgName, usrName, repoName).Run()
		if err != nil {
			fmt.Println("error running `docker tag`. Could not rename image for docker hub.")
		}

		err = docker.PushToHub(usrName, repoName).Run()
		if err != nil {
			fmt.Println("error running `docker push`. Could not push tag to docker hub.")
		}

	},
}
