package driver

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	valid "github.com/asaskevich/govalidator"
	"github.com/rancher/local-flexvolume/generator"
)

const (
	mountCmd               = "mount"
	unmountCmd             = "umount"
	svcLogBaseDir          = "/var/log/rancher-log-volumes"
	svcProjectLogConfigDir = "/fluentd/etc/config/customer/project"
	svcClusterLogConfigDir = "/fluentd/etc/config/customer/cluster"
)

type Options struct {
	ClusterName   string `json:"clusterName,omitempty" valid:"required"`
	ClusterID     string `json:"clusterID,omitempty" valid:"required"`
	ProjectName   string `json:"projectName,omitempty" valid:"required"`
	ProjectID     string `json:"projectID,omitempty" valid:"required"`
	Namespace     string `json:"namespace,omitempty" valid:"required"`
	WorkloadName  string `json:"workloadName,omitempty" valid:"required"`
	ContainerName string `json:"containerName,omitempty" valid:"required"`
	Format        string `json:"format,omitempty" valid:"required"`
	VolumeName    string `json:"volumeName,omitempty" valid:"required"`
}

var (
	mountCmdArg = []string{"-o", "bind"}
)

type FlexVolumeDriver struct {
	Logger *logrus.Logger
}

func (f *FlexVolumeDriver) Init() InitResponse {
	return InitResponse{
		CommonResponse: CommonResponse{
			Status:  StatusSuccess,
			Message: "Success",
		},
		Capabilities: struct {
			Attach bool `json:"attach"`
		}{
			Attach: false,
		},
	}
}

func (f *FlexVolumeDriver) Mount(args []string) CommonResponse {
	var err error
	defer func(logger *logrus.Logger) {
		if err != nil {
			logger.Error(err)
		}
	}(f.Logger)

	f.Logger.Debugf("mount args: %v", args)
	if err = checkArgsLen(args, 2); err != nil {
		return returnErrorResponse(err)
	}

	containerPath := args[0]
	opts := Options{}
	if err = json.Unmarshal([]byte(args[1]), &opts); err != nil {
		return returnErrorResponse(err)
	}

	if _, err = valid.ValidateStruct(opts); err != nil {
		return returnErrorResponse(err)
	}

	if _, err = os.Stat(svcLogBaseDir); os.IsNotExist(err) {
		if err = os.MkdirAll(svcLogBaseDir, 0755); err != nil {
			return returnErrorResponse(fmt.Errorf("create base dir %s failed, %v", svcLogBaseDir, err))
		}
	}

	fn := []string{opts.ClusterID, opts.ClusterName, opts.Namespace, opts.ProjectID, opts.ProjectName, opts.WorkloadName, opts.ContainerName, opts.VolumeName}
	generateDir := strings.Join(fn, "_")
	hostDir := path.Join(svcLogBaseDir, generateDir)
	if err = os.MkdirAll(hostDir, os.ModePerm); err != nil {
		return returnErrorResponse(fmt.Errorf("create hostPath failed, %v", err))
	}

	if err = os.MkdirAll(svcProjectLogConfigDir, os.ModePerm); err != nil {
		return returnErrorResponse(fmt.Errorf("create project config path %s failed, %v", svcProjectLogConfigDir, err))
	}

	if err = os.MkdirAll(svcClusterLogConfigDir, os.ModePerm); err != nil {
		return returnErrorResponse(fmt.Errorf("create cluster config path %s failed, %v", svcClusterLogConfigDir, err))
	}

	outputProjectPath := path.Join(svcProjectLogConfigDir, generateDir+".conf")
	outputClusterPath := path.Join(svcClusterLogConfigDir, generateDir+".conf")
	conf := map[string]interface{}{
		"Format":  opts.Format,
		"Path":    fmt.Sprintf("%s/*.*", hostDir),
		"Project": opts.ProjectID,
	}
	if err = generator.GenerateConfigFile(outputClusterPath, generator.ClusterSourceTemplate, "cluster", conf); err != nil {
		return returnErrorResponse(fmt.Errorf("generate cluster config file failed, %v", err))
	}

	if err = generator.GenerateConfigFile(outputProjectPath, generator.ProjectSourceTemplate, "project", conf); err != nil {
		return returnErrorResponse(fmt.Errorf("generate project config file failed, %v", err))
	}

	if err = bindMount(hostDir, containerPath); err != nil {
		return returnErrorResponse(fmt.Errorf("bind mount failed, %v", err))
	}
	return CommonResponse{
		Status:  StatusSuccess,
		Message: "Success",
	}
}

func (f *FlexVolumeDriver) Unmount(args []string) CommonResponse {
	var err error
	defer func(logger *logrus.Logger) {
		if err != nil {
			logger.Error(err)
		}
	}(f.Logger)

	f.Logger.Debugf("ummount args: %v", args)
	if err = checkArgsLen(args, 1); err != nil {
		return returnErrorResponse(err)
	}

	containerPath := args[0]
	if err = unMount(containerPath); err != nil {
		return returnErrorResponse(fmt.Errorf("unmount container path %s failed, %v", containerPath, err))
	}

	return CommonResponse{
		Status:  StatusSuccess,
		Message: "Success",
	}
}

func bindMount(hostPath string, containerPath string) error {
	c := append(mountCmdArg, hostPath)
	c = append(c, containerPath)
	cmd := exec.Command(mountCmd, c...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("run bind mount command failed, hostPath: %s, containerPath: %s, error: %v, output: %s", hostPath, containerPath, err, string(output))
	}
	return nil
}

func unMount(containerPath string) error {
	cmd := exec.Command(unmountCmd, containerPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf(string(output))
	}
	return nil
}

func checkArgsLen(args cli.Args, expectedNum int) error {
	if len(args) < expectedNum {
		err := fmt.Errorf("mount: invalid args num, %v", args)
		return err
	}
	return nil
}

func returnErrorResponse(err error) CommonResponse {
	return CommonResponse{
		Status:  StatusFailure,
		Message: fmt.Sprintf("%v", err),
	}
}
