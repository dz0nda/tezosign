package baseconf

import (
	"encoding/json"
	"io"
	"os"
	"reflect"
)

// BaseConfig is interface for all validatable structures
type BaseConfig interface {
	Validate() error
}

// Init loads config data from files or from ENV
func Init(cfg interface{}, filename *string) error {

	//check if file with config exists
	if _, err := os.Stat(*filename); os.IsNotExist(err) {
		//expected file with config not exists
		//check special /.secrets directory (DevOps special)
		developmentConfigPath := "./.secrets/config.json"
		if _, err := os.Stat(developmentConfigPath); os.IsNotExist(err) {
			return err
		}

		filename = &developmentConfigPath
	}

	file, err := os.Open(*filename)
	if err != nil {
		return err
	}

	return Load(cfg, file)
}

// Load loads config data from any reader or from ENV
func Load(cfg interface{}, source io.Reader) error {
	decoder := json.NewDecoder(source)
	err := decoder.Decode(&cfg)
	if err != nil {
		return err
	}

	return nil
}

// ValidateBaseConfigStructs validates additional structures (which implements BaseConfig)
func ValidateBaseConfigStructs(cfg interface{}) (err error) {
	v := reflect.ValueOf(cfg).Elem()
	baseConfigType := reflect.TypeOf((*BaseConfig)(nil)).Elem()

	for i := 0; i < v.NumField(); i++ {
		if v.Type().Field(i).Type.Implements(baseConfigType) {
			err = v.Field(i).Interface().(BaseConfig).Validate()
			if err != nil {
				return
			}
		}
	}

	return
}
