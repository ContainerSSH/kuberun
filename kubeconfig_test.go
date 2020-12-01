package kuberun_test

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os/user"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/containerssh/kuberun"
)

func createConfigFromKubeConfig() (config kuberun.Config, err error) {
	usr, err := user.Current()
	if err != nil {
		return config, err
	}
	kubectlConfig, err := readKubeConfig(filepath.Join(usr.HomeDir, ".kube", "config"))
	if err != nil {
		return config, fmt.Errorf("failed to read kubeconfig (%w)", err)
	}
	context := extractKubeConfigContext(kubectlConfig, kubectlConfig.CurrentContext)
	if context == nil {
		return config, fmt.Errorf("failed to find current kubeConfigContext in kubeConfig")
	}

	kubeConfigUser := extractKubeConfigUser(kubectlConfig, context.Context.User)
	if kubeConfigUser == nil {
		return config, fmt.Errorf("failed to find kubeConfigUser in kubeConfig")
	}

	kubeConfigCluster := extractKubeConfigCluster(kubectlConfig, context.Context.Cluster)
	if kubeConfigCluster == nil {
		return config, fmt.Errorf("failed to find kubeConfigCluster in kubeConfig")
	}

	config.Connection.Host = strings.Replace(
		kubeConfigCluster.Cluster.Server,
		"https://",
		"",
		1,
	)
	if err = configureCertificates(kubeConfigCluster, kubeConfigUser, &config); err != nil {
		return config, err
	}

	return config, nil
}

func extractKubeConfigContext(kubectlConfig kubeConfig, currentContext string) *kubeConfigContext {
	var context *kubeConfigContext
	for _, ctx := range kubectlConfig.Contexts {
		if ctx.Name == currentContext {
			context = &ctx
			break
		}
	}
	return context
}

func configureCertificates(
	kubeConfigCluster *kubeConfigCluster,
	kubeConfigUser *kubeConfigUser,
	config *kuberun.Config,
) error {
	decodedCa, err := base64.StdEncoding.DecodeString(
		kubeConfigCluster.Cluster.CertificateAuthorityData,
	)
	if err != nil {
		return err
	}
	config.Connection.CAData = string(decodedCa)

	decodedKey, err := base64.StdEncoding.DecodeString(
		kubeConfigUser.User.ClientKeyData,
	)
	if err != nil {
		return err
	}
	config.Connection.KeyData = string(decodedKey)

	decodedCert, err := base64.StdEncoding.DecodeString(
		kubeConfigUser.User.ClientCertificateData,
	)
	if err != nil {
		return err
	}
	config.Connection.CertData = string(decodedCert)
	return nil
}

func extractKubeConfigCluster(kubectlConfig kubeConfig, clusterName string) *kubeConfigCluster {
	var kubeConfigCluster *kubeConfigCluster
	for _, c := range kubectlConfig.Clusters {
		if c.Name == clusterName {
			kubeConfigCluster = &c
			break
		}
	}
	return kubeConfigCluster
}

func extractKubeConfigUser(kubectlConfig kubeConfig, userName string) *kubeConfigUser {
	var kubeConfigUser *kubeConfigUser
	for _, u := range kubectlConfig.Users {
		if u.Name == userName {
			kubeConfigUser = &u
			break
		}
	}
	return kubeConfigUser
}

type kubeConfig struct {
	ApiVersion     string              `yaml:"apiVersion" default:"v1"`
	Clusters       []kubeConfigCluster `yaml:"clusters"`
	Contexts       []kubeConfigContext `yaml:"contexts"`
	CurrentContext string              `yaml:"current-kubeConfigContext"`
	Kind           string              `yaml:"kind" default:"Config"`
	Preferences    map[string]string   `yaml:"preferences"`
	Users          []kubeConfigUser    `yaml:"users"`
}

type kubeConfigCluster struct {
	Name    string `yaml:"name"`
	Cluster struct {
		CertificateAuthorityData string `yaml:"certificate-authority-data"`
		Server                   string `yaml:"server"`
	} `yaml:"kubeConfigCluster"`
}

type kubeConfigContext struct {
	Name    string `yaml:"name"`
	Context struct {
		Cluster string `yaml:"kubeConfigCluster"`
		User    string `yaml:"kubeConfigUser"`
	} `yaml:"kubeConfigContext"`
}

type kubeConfigUser struct {
	Name string `yaml:"name"`
	User struct {
		ClientCertificateData string `yaml:"client-certificate-data"`
		ClientKeyData         string `yaml:"client-key-data"`
	} `yaml:"kubeConfigUser"`
}

func readKubeConfig(file string) (config kubeConfig, err error) {
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return config, err
	}
	return config, nil
}
