package main

import (
	"encoding/json"
	"io/ioutil"
)

type CosSaverCfg struct {
	SecretID          string   `json:"secretID"`
	SercretKey        string   `json:"sercretKey"`
	BucketURL         string   `json:"bucketURL"`
	DefaultUploadTime string   `json:"defaultUploadTime"`
	SourceDir         string   `json:"sourceDir"`
	InitKeyDir        string   `json:"initKeyDir"`
	SkippedDir        []string `json:"skippedDir"`
	Action            string   `json:"action"`
}

func loadConfig(cfgFile string) (cfg *CosSaverCfg, err error) {
	content, err := ioutil.ReadFile(cfgFile)
	if err != nil {
		return nil, err
	}

	cfg = &CosSaverCfg{}
	if err = json.Unmarshal(content, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
