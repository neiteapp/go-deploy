package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"neite.dev/go-ship/internal/config"
	"neite.dev/go-ship/internal/docker"
	"neite.dev/go-ship/internal/lockfile"
	"neite.dev/go-ship/internal/ssh"
)

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

		commitHash, err := latestCommitHash()
		if err != nil {
			fmt.Println(err)
			return
		}

		imgName := fmt.Sprintf("%s:%s", userCfg.Image, commitHash)

		err = docker.BuildImage(imgName, userCfg.Dockerfile).Run()
		if err != nil {
			fmt.Println("Error running `docker build`. Could not build your image.")
			fmt.Println(err)
			return
		}

		err = docker.Tag(imgName, userCfg.Registry.Server).Run()
		if err != nil {
			fmt.Println("Error running `docker tag`.")
			fmt.Println(err)
			return
		}

		err = docker.PushToHub(fmt.Sprintf("%s/%s:%s", userCfg.Registry.Server, userCfg.Image, commitHash)).Run()
		if err != nil {
			fmt.Println("error running `docker push`. Could not push tag to docker hub.")
			return
		}

		// setup connection with server
		client, err := ssh.NewConnection(userCfg.Servers[0], userCfg.SSH.Port)
		if err != nil {
			fmt.Println("error establishing connection with server.")
			fmt.Println(err)
			return
		}
		defer client.Close()

		fmt.Println("Connected to server")

		err = docker.IsInstalled().RunSSH(client)
		if err != nil {
			switch {
			// command `docker` could not be found meaning the docker is not installed
			case errors.Is(err, ssh.ErrExit):
				fmt.Println("docker is not installed; installing...")
				err := installDocker(client)
				if err != nil {
					fmt.Println("could not install docker on your server")
					return
				}
			default:
				fmt.Println("could not check if docker is intsalled on the server")
				return
			}
		}

		err = docker.PullFromHub(fmt.Sprintf("%s/%s:%s", userCfg.Registry.Server, userCfg.Image, commitHash)).RunSSH(client)
		if err != nil {
			fmt.Println("could not pull your image from DockerHub on the server")
			return
		}

		// because it's the setup we can run container instead of starting or restarting it

		imgWithRegistry := fmt.Sprintf("%s/%s", userCfg.Registry.Server, imgName)
		err = docker.RunContainer(3000, userCfg.Service, imgWithRegistry).RunSSH(client)
		if err != nil {
			fmt.Println("could not run your container on the server")
			return
		}

		f, err := lockfile.CreateLockFile()
		if err != nil {
			fmt.Println("could not create lockfile")
			return
		}
		defer f.Close()

		commitMsg, err := latestCommitMsg()
		if err != nil {
			fmt.Println(err)
			return
		}

		data := lockfile.LockVersion{
			Version: commitHash,
			Image:   imgName,
			Message: commitMsg,
			Date:    now(),
		}

		if err := lockfile.Write(f, data); err != nil {
			fmt.Printf("could not write to lockfile\n. Error: %s", err)
			return
		}

	},
}

func installDocker(client *ssh.Client) error {
	sftpClient, err := client.NewSFTPClient()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	err = sftpClient.TransferExecutable("./scripts/setup.sh", "setup.sh")
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	if err := session.Run("./setup.sh"); err != nil {
		return err
	}
	return nil
}
