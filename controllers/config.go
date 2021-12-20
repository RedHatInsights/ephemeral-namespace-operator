package controllers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	frontend "github.com/RedHatInsights/frontend-operator/api/v1alpha1"
	core "k8s.io/api/core/v1"
)

type OperatorConfig struct {
	PoolConfig      PoolConfig                       `json:"pool"`
	ClowdEnvSpec    clowder.ClowdEnvironmentSpec     `json:"clowdEnv"`
	FrontendEnvSpec frontend.FrontendEnvironmentSpec `json:"frontendEnv"`
	LimitRange      core.LimitRange                  `json:"limitRange"`
	ResourceQuotas  core.ResourceQuotaList           `json:"resourceQuotas"`
}

type PoolConfig struct {
	Size  int  `json:"size"`
	Local bool `json:"local"`
}

func getConfig() OperatorConfig {
	configPath := "ephemeral_config.json"

	if path := os.Getenv("NS_OPERATOR_CONFIG"); path != "" {
		configPath = path
	}

	fmt.Printf("Loading config from: %s\n", configPath)

	jsonData, err := ioutil.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Config file %s not found\n", configPath)
		return OperatorConfig{}
	}

	operatorConfig := OperatorConfig{}
	err = json.Unmarshal(jsonData, &operatorConfig)
	if err != nil {
		fmt.Printf("Couldn't parse json:\n" + err.Error())
		return OperatorConfig{}
	}

	return operatorConfig
}

var LoadedOperatorConfig OperatorConfig

func init() {
	LoadedOperatorConfig = getConfig()
}
