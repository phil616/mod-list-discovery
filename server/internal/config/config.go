package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server struct {
		Listen              string `yaml:"listen"`
		PublicBaseURL       string `yaml:"public_base_url"`
		DataDir             string `yaml:"data_dir"`
		TrustedReverseProxy bool   `yaml:"trusted_reverse_proxy"`
	} `yaml:"server"`
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	Security struct {
		SessionTTL             string `yaml:"session_ttl"`
		EnrollmentTokenTTL     string `yaml:"enrollment_token_ttl"`
		ArtifactURLTTL         string `yaml:"artifact_url_ttl"`
		MaxUploadSize          int64  `yaml:"max_upload_size"`
		AllowPrivateImportURLs bool   `yaml:"allow_private_import_urls"`
	} `yaml:"security"`
	Storage struct {
		Type  string `yaml:"type"`
		Local struct {
			Root string `yaml:"root"`
		} `yaml:"local"`
		S3  Remote `yaml:"s3"`
		OSS Remote `yaml:"oss"`
	} `yaml:"storage"`
}
type Remote struct {
	Endpoint       string `yaml:"endpoint"`
	Region         string `yaml:"region"`
	Bucket         string `yaml:"bucket"`
	Prefix         string `yaml:"prefix"`
	AccessKeyEnv   string `yaml:"access_key_env"`
	SecretKeyEnv   string `yaml:"secret_key_env"`
	ForcePathStyle bool   `yaml:"force_path_style"`
}

func Load(path string) (Config, error) {
	var c Config
	c.Server.Listen = "127.0.0.1:8080"
	c.Server.PublicBaseURL = "http://127.0.0.1:8080"
	c.Server.DataDir = "./data"
	c.Security.SessionTTL = "12h"
	c.Security.EnrollmentTokenTTL = "24h"
	c.Security.ArtifactURLTTL = "5m"
	c.Security.MaxUploadSize = 536870912
	c.Storage.Type = "local"
	c.Storage.S3 = Remote{Region: "us-east-1", Prefix: "mod-list-discovery/", AccessKeyEnv: "MLD_S3_ACCESS_KEY", SecretKeyEnv: "MLD_S3_SECRET_KEY"}
	c.Storage.OSS = Remote{Prefix: "mod-list-discovery/", AccessKeyEnv: "MLD_OSS_ACCESS_KEY", SecretKeyEnv: "MLD_OSS_SECRET_KEY"}
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	if e = yaml.Unmarshal(b, &c); e != nil {
		return c, e
	}
	if e = override(reflect.ValueOf(&c).Elem(), "MLD"); e != nil {
		return c, e
	}
	if c.Database.Path == "" {
		c.Database.Path = filepath.Join(c.Server.DataDir, "mld.sqlite3")
	}
	if c.Storage.Local.Root == "" {
		c.Storage.Local.Root = filepath.Join(c.Server.DataDir, "artifacts")
	}
	u, e := url.Parse(c.Server.PublicBaseURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && u.Scheme != "http") {
		return c, fmt.Errorf("invalid public_base_url")
	}
	for _, s := range []string{c.Security.SessionTTL, c.Security.EnrollmentTokenTTL, c.Security.ArtifactURLTTL} {
		d, e := time.ParseDuration(s)
		if e != nil || d <= 0 {
			return c, fmt.Errorf("invalid duration")
		}
	}
	if c.Security.MaxUploadSize <= 0 {
		return c, fmt.Errorf("invalid upload limit")
	}
	return c, nil
}
func override(v reflect.Value, prefix string) error {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		key := prefix + "_" + strings.ToUpper(t.Field(i).Tag.Get("yaml"))
		if f.Kind() == reflect.Struct {
			if e := override(f, key); e != nil {
				return e
			}
			continue
		}
		s, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		switch f.Kind() {
		case reflect.String:
			f.SetString(s)
		case reflect.Bool:
			b, e := strconv.ParseBool(s)
			if e != nil {
				return e
			}
			f.SetBool(b)
		case reflect.Int64:
			n, e := strconv.ParseInt(s, 10, 64)
			if e != nil {
				return e
			}
			f.SetInt(n)
		}
	}
	return nil
}
func Duration(s string) time.Duration { d, _ := time.ParseDuration(s); return d }
