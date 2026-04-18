package config

import (
	"fmt"
	"log"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	MQTT    MQTTConfig    `mapstructure:"mqtt"`
	AMQP    AMQPConfig    `mapstructure:"amqp"`
	IOTAPI  IOTAPIConfig  `mapstructure:"iot_api"`
	Storage StorageConfig `mapstructure:"storage"`
	Alarm   AlarmConfig   `mapstructure:"alarm"`
	Logging LoggingConfig `mapstructure:"logging"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type MQTTConfig struct {
	Broker                 string        `mapstructure:"broker"`
	Port                   int           `mapstructure:"port"`
	ClientID               string        `mapstructure:"client_id"`
	Username               string        `mapstructure:"username"`
	Password               string        `mapstructure:"password"`
	SubscribeTopic         string        `mapstructure:"subscribe_topic"`
	SubscribeTopicWildcard string        `mapstructure:"subscribe_topic_wildcard"`
	PublishTopic           string        `mapstructure:"publish_topic"`
	QoS                    byte          `mapstructure:"qos"`
	KeepAlive              time.Duration `mapstructure:"keep_alive"`
	PingTimeout            time.Duration `mapstructure:"ping_timeout"`
	MaxReconnectInterval   time.Duration `mapstructure:"max_reconnect_interval"`
}

type AMQPConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	AccessKeySecret string `mapstructure:"access_key_secret"`
	ConsumerGroupID string `mapstructure:"consumer_group_id"`
	IOTInstanceID   string `mapstructure:"iot_instance_id"`
}

type IOTAPIConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	AccessKeySecret string `mapstructure:"access_key_secret"`
	Region          string `mapstructure:"region"`
	InstanceID      string `mapstructure:"instance_id"`
	ProductKey      string `mapstructure:"product_key"`
	DeviceName      string `mapstructure:"device_name"`
	PollInterval    int    `mapstructure:"poll_interval"`
}

type StorageConfig struct {
	DataDir       string `mapstructure:"data_dir"`
	RetentionDays int    `mapstructure:"retention_days"`
}

type AlarmConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	CheckInterval time.Duration `mapstructure:"check_interval"`
}

type LoggingConfig struct {
	Level string `mapstructure:"level"`
	Path  string `mapstructure:"path"`
}

var GlobalConfig *Config

func LoadConfig(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	GlobalConfig = &config
	log.Printf("配置加载成功，服务器端口: %d", config.Server.Port)
	return &config, nil
}
