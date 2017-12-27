package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	_ "expvar"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"text/template"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vaughan0/go-ini"
)

const (
	FileOutputType = iota
	S3OutputType
	TCPOutputType
	UDPOutputType
	SyslogOutputType
	HttpOutputType
	SplunkOutputType
	KafkaOutputType
)

const (
	LEEFOutputFormat = iota
	JSONOutputFormat
)

type Configuration struct {
	ServerName           string
	AMQPHostname         string
	DebugFlag            bool
	DebugStore           string
	OutputType           int
	OutputFormat         int
	AMQPUsername         string
	AMQPPassword         string
	AMQPPort             int
	AMQPTLSEnabled       bool
	AMQPTLSClientKey     string
	AMQPTLSClientCert    string
	AMQPTLSCACert        string
	AMQPQueueName        string
	AMQPAutoDeleteQueue  bool
	OutputParameters     string
	EventTypes           []string
	EventMap             map[string]bool
	HTTPServerPort       int
	CbServerURL          string
	UseRawSensorExchange bool
	MonitoredLogs        []string

	// this is a hack for S3 specific configuration
	S3ServerSideEncryption  *string
	S3CredentialProfileName *string
	S3ACLPolicy             *string
	S3ObjectPrefix          *string
	S3VerboseKey            bool
	S3CompressData          bool

	// SSL/TLS-specific configuration
	TLSClientKey  *string
	TLSClientCert *string
	TLSCACert     *string
	TLSVerify     bool
	TLSCName      *string
	TLS12Only     bool

	// HTTP-specific configuration
	HttpAuthorizationToken *string
	HttpPostTemplate       *template.Template
	HttpContentType        *string

	// configuration options common to bundled outputs (S3, HTTP)
	UploadEmptyFiles    bool
	CommaSeparateEvents bool
	BundleSendTimeout   time.Duration
	BundleSizeMax       int64

	// Compress data on S3 or file output types
	FileHandlerCompressData bool

	TLSConfig *tls.Config

	// optional post processing of feed hits to retrieve titles
	PerformFeedPostprocessing bool
	CbAPIToken                string
	CbAPIVerifySSL            bool
	CbAPIProxyUrl             string

	// Kafka-specific configuration
	KafkaBrokers     *string
	KafkaTopicSuffix *string

	// Audit redis configuration
	AuditingEnabled          bool
	AuditRedisHost           string
	AuditRedisDatabaseNumber int
	AuditPipelineSize        int

	//Splunkd
	SplunkToken *string
	AuditLog    bool
}

type ConfigurationError struct {
	Errors []string
	Empty  bool
}

func (c *Configuration) AMQPURL() string {
	return fmt.Sprintf("amqp://%s:%s@%s:%d", c.AMQPUsername, c.AMQPPassword, c.AMQPHostname, c.AMQPPort)
}

func (e ConfigurationError) Error() string {
	return fmt.Sprintf("Configuration errors:\n %s", strings.Join(e.Errors, "\n "))
}

func (e *ConfigurationError) addErrorString(err string) {
	e.Empty = false
	e.Errors = append(e.Errors, err)
}

func (e *ConfigurationError) addError(err error) {
	e.Empty = false
	e.Errors = append(e.Errors, err.Error())
}

func parseCbConf() (username, password string, err error) {
	input, err := ini.LoadFile("/etc/cb/cb.conf")
	if err != nil {
		return username, password, err
	}
	username, _ = input.Get("", "RabbitMQUser")
	password, _ = input.Get("", "RabbitMQPassword")

	if len(username) == 0 || len(password) == 0 {
		return username, password, errors.New("Could not get RabbitMQ credentials from /etc/cb/cb.conf")
	}
	return
}

func (c *Configuration) parseEventTypes(input ini.File) {
	eventTypes := [...]struct {
		configKey string
		eventList []string
	}{
		{"events_watchlist", []string{
			"watchlist.#",
		}},
		{"events_feed", []string{
			"feed.#",
		}},
		{"events_alert", []string{
			"alert.#",
		}},
		{"events_raw_sensor", []string{
			"ingress.event.process",
			"ingress.event.procstart",
			"ingress.event.netconn",
			"ingress.event.procend",
			"ingress.event.childproc",
			"ingress.event.moduleload",
			"ingress.event.module",
			"ingress.event.filemod",
			"ingress.event.regmod",
			"ingress.event.tamper",
			"ingress.event.crossprocopen",
			"ingress.event.remotethread",
			"ingress.event.processblock",
			"ingress.event.emetmitigation",
		}},
		{"events_binary_observed", []string{
			"binaryinfo.#",
		}},
		{"events_binary_upload", []string{
			"binarystore.#",
		}},
		{"events_storage_partition", []string{
			"events.partition.#",
		}},
	}

	for _, eventType := range eventTypes {
		val, ok := input.Get("bridge", eventType.configKey)
		if ok {
			val = strings.ToLower(val)
			if val == "all" {
				for _, routingKey := range eventType.eventList {
					c.EventTypes = append(c.EventTypes, routingKey)
				}
			} else if val == "0" {
				// nothing
			} else {
				for _, routingKey := range strings.Split(val, ",") {
					c.EventTypes = append(c.EventTypes, routingKey)
				}
			}
		}
	}

	c.EventMap = make(map[string]bool)

	log.Info("Raw Event Filtering Configuration:")
	for _, eventName := range c.EventTypes {
		c.EventMap[eventName] = true
		if strings.HasPrefix(eventName, "ingress.event.") {
			log.Infof("%s: %t", eventName, c.EventMap[eventName])
		}
	}
}

func (c *Configuration) parseMonitoredLogs(input ini.File) {
	val, ok := input.Get("bridge", "monitored_logs")
	if ok {
		for _, monitored_log := range strings.Split(val, ",") {
			c.MonitoredLogs = append(c.MonitoredLogs, monitored_log)
		}
	}
}

func ParseConfig(fn string) (Configuration, error) {
	config := Configuration{}
	errs := ConfigurationError{Empty: true}

	input, err := ini.LoadFile(fn)
	if err != nil {
		return config, err
	}

	// defaults
	config.DebugFlag = false
	config.OutputFormat = JSONOutputFormat
	config.OutputType = FileOutputType
	config.AMQPHostname = "localhost"
	config.AMQPUsername = "cb"
	config.HTTPServerPort = 33706
	config.AMQPPort = 5004
	config.DebugStore = "/tmp"

	config.S3ACLPolicy = nil
	config.S3ServerSideEncryption = nil
	config.S3CredentialProfileName = nil
	config.AMQPAutoDeleteQueue = true

	// required values
	val, ok := input.Get("bridge", "server_name")
	if !ok {
		config.ServerName = "CB"
	} else {
		config.ServerName = val
	}

	val, ok = input.Get("bridge", "debug")
	if ok {
		if val == "1" {
			config.DebugFlag = true
			log.SetLevel(log.DebugLevel)

			customFormatter := new(log.TextFormatter)
			customFormatter.TimestampFormat = "2006-01-02 15:04:05"
			log.SetFormatter(customFormatter)
			customFormatter.FullTimestamp = true

			log.Debug("Debugging output is set to True")
		}
	}

	debugStore, ok := input.Get("bridge", "debug_store")
	if ok {
		config.DebugStore = debugStore
	} else {
		config.DebugStore = "/var/log/cb/integrations/cb-event-forwarder"
	}

	log.Debugf("Debug Store is %s", config.DebugStore)

	val, ok = input.Get("bridge", "http_server_port")
	if ok {
		port, err := strconv.Atoi(val)
		if err == nil {
			config.HTTPServerPort = port
		}
	}

	val, ok = input.Get("bridge", "rabbit_mq_username")
	if ok {
		config.AMQPUsername = val
	}

	val, ok = input.Get("bridge", "rabbit_mq_password")
	if !ok {
		errs.addErrorString("Missing required rabbit_mq_password section")
	} else {
		config.AMQPPassword = val
	}

	val, ok = input.Get("bridge", "rabbit_mq_port")
	if ok {
		port, err := strconv.Atoi(val)
		if err == nil {
			config.AMQPPort = port
		}
	}

	val, ok = input.Get("bridge", "rabbit_mq_auto_delete_queue")
	if ok {
		b, err := strconv.ParseBool(val)
		if err == nil {
			config.AMQPAutoDeleteQueue = b
		}
	}

	if len(config.AMQPUsername) == 0 || len(config.AMQPPassword) == 0 {
		config.AMQPUsername, config.AMQPPassword, err = parseCbConf()
		if err != nil {
			errs.addError(err)
		}
	}

	val, ok = input.Get("bridge", "rabbit_mq_use_tls")
	if ok {
		if ok {
			b, err := strconv.ParseBool(val)
			if err == nil {
				config.AMQPTLSEnabled = b
			}
		}
	}

	rabbitKeyFilename, ok := input.Get("bridge", "rabbit_mq_key")
	if ok {
		config.AMQPTLSClientKey = rabbitKeyFilename
	}

	rabbitCertFilename, ok := input.Get("bridge", "rabbit_mq_cert")
	if ok {
		config.AMQPTLSClientCert = rabbitCertFilename
	}

	rabbitCaCertFilename, ok := input.Get("bridge", "rabbit_mq_ca_cert")
	if ok {
		config.AMQPTLSCACert = rabbitCaCertFilename
	}

	rabbitQueueName, ok := input.Get("bridge", "rabbit_mq_queue_name")
	if ok {
		config.AMQPQueueName = rabbitQueueName
	}

	val, ok = input.Get("bridge", "cb_server_hostname")
	if ok {
		config.AMQPHostname = val
	}

	val, ok = input.Get("bridge", "cb_server_url")
	if ok {
		if !strings.HasSuffix(val, "/") {
			val = val + "/"
		}
		config.CbServerURL = val
	}

	val, ok = input.Get("bridge", "output_format")
	if ok {
		val = strings.TrimSpace(val)
		val = strings.ToLower(val)
		if val == "leef" {
			config.OutputFormat = LEEFOutputFormat
		}
	}

	config.FileHandlerCompressData = false
	val, ok = input.Get("bridge", "compress_data")
	if ok {
		b, err := strconv.ParseBool(val)
		if err == nil {
			config.FileHandlerCompressData = b
		}
	}

	config.AuditLog = false
	val, ok = input.Get("bridge", "audit_log")
	if ok {
		b, err := strconv.ParseBool(val)
		if err == nil {
			config.AuditLog = b
		}
	}

	outType, ok := input.Get("bridge", "output_type")
	var parameterKey string
	if ok {
		outType = strings.TrimSpace(outType)
		outType = strings.ToLower(outType)

		switch outType {
		case "file":
			parameterKey = "outfile"
			config.OutputType = FileOutputType
		case "tcp":
			parameterKey = "tcpout"
			config.OutputType = TCPOutputType
		case "udp":
			parameterKey = "udpout"
			config.OutputType = UDPOutputType
		case "s3":
			parameterKey = "s3out"
			config.OutputType = S3OutputType

			profileName, ok := input.Get("s3", "credential_profile")
			if ok {
				config.S3CredentialProfileName = &profileName
			}

			aclPolicy, ok := input.Get("s3", "acl_policy")
			if ok {
				config.S3ACLPolicy = &aclPolicy
			}

			sseType, ok := input.Get("s3", "server_side_encryption")
			if ok {
				config.S3ServerSideEncryption = &sseType
			}

			objectPrefix, ok := input.Get("s3", "object_prefix")
			if ok {
				config.S3ObjectPrefix = &objectPrefix
			}

			val, ok = input.Get("s3", "verbose_key")
			if ok {
				b, err := strconv.ParseBool(val)
				if err == nil {
					config.S3VerboseKey = b
				}
			}

			val, ok = input.Get("s3", "compress_data")
			if ok {
				b, err := strconv.ParseBool(val)
				if err == nil {
					config.S3CompressData = b
				}
			} else {
				config.S3CompressData = true
			}
		case "http":
			parameterKey = "httpout"
			config.OutputType = HttpOutputType

			token, ok := input.Get("http", "authorization_token")
			if ok {
				config.HttpAuthorizationToken = &token
			}

			postTemplate, ok := input.Get("http", "http_post_template")
			config.HttpPostTemplate = template.New("http_post_output")
			if ok {
				config.HttpPostTemplate = template.Must(config.HttpPostTemplate.Parse(postTemplate))
			} else {
				if config.OutputFormat == JSONOutputFormat {
					config.HttpPostTemplate = template.Must(config.HttpPostTemplate.Parse(
						`{"filename": "{{.FileName}}", "service": "carbonblack", "alerts":[{{range .Events}}{{.EventText}}{{end}}]}`))
				} else {
					config.HttpPostTemplate = template.Must(config.HttpPostTemplate.Parse(`{{range .Events}}{{.EventText}}{{end}}`))
				}
			}

			contentType, ok := input.Get("http", "content_type")
			if ok {
				config.HttpContentType = &contentType
			} else {
				jsonString := "application/json"
				config.HttpContentType = &jsonString
			}
		case "syslog":
			parameterKey = "syslogout"
			config.OutputType = SyslogOutputType
		case "kafka":
			config.OutputType = KafkaOutputType

			kafkaBrokers, ok := input.Get("kafka", "brokers")
			if ok {
				config.KafkaBrokers = &kafkaBrokers
			}

			kafkaTopicSuffix, ok := input.Get("kafka", "topic_suffix")
			if ok {
				config.KafkaTopicSuffix = &kafkaTopicSuffix
			}
		case "splunk":
			parameterKey = "splunkout"
			config.OutputType = SplunkOutputType

			token, ok := input.Get("splunk", "hec_token")
			if ok {
				config.SplunkToken = &token
			}

			postTemplate, ok := input.Get("splunk", "http_post_template")
			config.HttpPostTemplate = template.New("http_post_output")
			if ok {
				config.HttpPostTemplate = template.Must(config.HttpPostTemplate.Parse(postTemplate))
			} else {
				if config.OutputFormat == JSONOutputFormat {
					config.HttpPostTemplate = template.Must(config.HttpPostTemplate.Parse(
						`{{range .Events}}{"sourcetype":"bit9:carbonblack:json","event":{{.EventText}}}{{end}}`))
				} else {
					config.HttpPostTemplate = template.Must(config.HttpPostTemplate.Parse(`{{range .Events}}{{.EventText}}{{end}}`))
				}
			}

			contentType, ok := input.Get("http", "content_type")
			if ok {
				config.HttpContentType = &contentType
			} else {
				jsonString := "application/json"
				config.HttpContentType = &jsonString
			}

		default:
			errs.addErrorString(fmt.Sprintf("Unknown output type: %s", outType))
		}
	} else {
		errs.addErrorString("No output type specified")
		return config, errs
	}

	val, ok = input.Get("audit", "enabled")
	log.Println("Auditing Enabled: ", val)
	if ok {
		b, err := strconv.ParseBool(val)
		if err == nil {
			config.AuditingEnabled = b
		}
	}

	if config.AuditingEnabled == true {
		val, ok = input.Get("audit", "redis_host")
		log.Println("HOST: ", val)
		if ok {
			log.Println("HOST: ", val)
			config.AuditRedisHost = val
		} else {
			log.Panic("NOT OK")
		}

		val, ok = input.Get("audit", "redis_database_number")
		if ok {
			db_number, err := strconv.Atoi(val)
			if err == nil {
				config.AuditRedisDatabaseNumber = db_number
			}
		}

		val, ok = input.Get("audit", "pipeline_size")
		if ok {
			pipeline_size, err := strconv.Atoi(val)
			if err == nil {
				config.AuditPipelineSize = pipeline_size
			}
		}
	}

	if len(parameterKey) > 0 {
		val, ok = input.Get("bridge", parameterKey)
		if !ok {
			errs.addErrorString(fmt.Sprintf("Missing value for key %s, required by output type %s",
				parameterKey, outType))
		} else {
			config.OutputParameters = val
		}
	}

	val, ok = input.Get("bridge", "use_raw_sensor_exchange")
	if ok {
		boolval, err := strconv.ParseBool(val)
		if err == nil {
			config.UseRawSensorExchange = boolval
			if boolval {
				log.Warn("Configured to listen on the Carbon Black Enterprise Response raw sensor event feed.")
				log.Warn("- This will result in a *large* number of messages output via the event forwarder!")
				log.Warn("- Ensure that raw sensor events are enabled in your Cb server (master & minion) via")
				log.Warn("  the 'EnableRawSensorDataBroadcast' variable in /etc/cb/cb.conf")
			}
		} else {
			errs.addErrorString("Unknown value for 'use_raw_sensor_exchange': valid values are true, false, 1, 0")
		}
	}

	// TLS configuration
	clientKeyFilename, ok := input.Get(outType, "client_key")
	if ok {
		config.TLSClientKey = &clientKeyFilename
	}

	clientCertFilename, ok := input.Get(outType, "client_cert")
	if ok {
		config.TLSClientCert = &clientCertFilename
	}

	caCertFilename, ok := input.Get(outType, "ca_cert")
	if ok {
		config.TLSCACert = &caCertFilename
	}

	config.TLSVerify = true
	tlsVerify, ok := input.Get(outType, "tls_verify")
	if ok {
		boolval, err := strconv.ParseBool(tlsVerify)
		if err == nil {
			if boolval == false {
				config.TLSVerify = false
			}
		} else {
			errs.addErrorString("Unknown value for 'tls_verify': valid values are true, false, 1, 0. Default is 'true'")
		}
	}

	config.TLS12Only = true
	tlsInsecure, ok := input.Get(outType, "insecure_tls")
	if ok {
		boolval, err := strconv.ParseBool(tlsInsecure)
		if err == nil {
			if boolval == true {
				config.TLS12Only = false
			}
		} else {
			errs.addErrorString("Unknown value for 'insecure_tls': ")
		}
	}

	serverCName, ok := input.Get(outType, "server_cname")
	if ok {
		config.TLSCName = &serverCName
	}

	config.TLSConfig = configureTLS(config)

	// Bundle configuration

	// default to sending empty files to S3/HTTP POST endpoint
	if outType == "splunk" {
		config.UploadEmptyFiles = false
		log.Info("Splunk HEC does not accept empty files as input, ignoring upload_empty_files=true for 'splunkout'")
	} else {
		config.UploadEmptyFiles = true
	}
	sendEmptyFiles, ok := input.Get(outType, "upload_empty_files")
	if ok {
		boolval, err := strconv.ParseBool(sendEmptyFiles)
		if err == nil {
			if boolval == false {
				config.UploadEmptyFiles = false
			}
		} else {
			errs.addErrorString("Unknown value for 'upload_empty_files': valid values are true, false, 1, 0. Default is 'true'")
		}
	}

	if config.OutputFormat == JSONOutputFormat {
		config.CommaSeparateEvents = true
	} else {
		config.CommaSeparateEvents = false
	}

	// default 10MB bundle size max before forcing a send
	config.BundleSizeMax = 10 * 1024 * 1024
	bundleSizeMax, ok := input.Get(outType, "bundle_size_max")
	if ok {
		bundleSizeMax, err := strconv.ParseInt(bundleSizeMax, 10, 64)
		if err == nil {
			config.BundleSizeMax = bundleSizeMax
		}
	}

	// default 5 minute send interval
	config.BundleSendTimeout = 5 * time.Minute
	bundleSendTimeout, ok := input.Get(outType, "bundle_send_timeout")
	if ok {
		bundleSendTimeout, err := strconv.ParseInt(bundleSendTimeout, 10, 64)
		if err == nil {
			config.BundleSendTimeout = time.Duration(bundleSendTimeout) * time.Second
		}
	}

	val, ok = input.Get("bridge", "api_verify_ssl")
	if ok {
		config.CbAPIVerifySSL, err = strconv.ParseBool(val)
		if err != nil {
			errs.addErrorString("Unknown value for 'api_verify_ssl': valid values are true, false, 1, 0. Default is 'false'")
		}
	}
	val, ok = input.Get("bridge", "api_token")
	if ok {
		config.CbAPIToken = val
		config.PerformFeedPostprocessing = true
	}

	config.CbAPIProxyUrl = ""
	val, ok = input.Get("bridge", "api_proxy_url")
	if ok {
		config.CbAPIProxyUrl = val
	}

	config.parseEventTypes(input)

	config.parseMonitoredLogs(input)

	if !errs.Empty {
		return config, errs
	} else {
		return config, nil
	}
}

func configureTLS(config Configuration) *tls.Config {
	tlsConfig := &tls.Config{}

	if config.TLSVerify == false {
		log.Info("Disabling TLS verification for remote output")
		tlsConfig.InsecureSkipVerify = true
	}

	if config.TLSClientCert != nil && config.TLSClientKey != nil && len(*config.TLSClientCert) > 0 &&
		len(*config.TLSClientKey) > 0 {
		log.Infof("Loading client cert/key from %s & %s", *config.TLSClientCert, *config.TLSClientKey)
		cert, err := tls.LoadX509KeyPair(*config.TLSClientCert, *config.TLSClientKey)
		if err != nil {
			log.Fatal(err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if config.TLSCACert != nil && len(*config.TLSCACert) > 0 {
		// Load CA cert
		log.Infof("Loading valid CAs from file %s", *config.TLSCACert)
		caCert, err := ioutil.ReadFile(*config.TLSCACert)
		if err != nil {
			log.Fatal(err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	if config.TLSCName != nil && len(*config.TLSCName) > 0 {
		log.Infof("Forcing TLS Common Name check to use '%s' as the hostname", *config.TLSCName)
		tlsConfig.ServerName = *config.TLSCName
	}

	if config.TLS12Only == true {
		log.Info("Enforcing minimum TLS version 1.2")
		tlsConfig.MinVersion = tls.VersionTLS12
	} else {
		log.Info("Relaxing minimum TLS version to 1.0")
		tlsConfig.MinVersion = tls.VersionTLS10
	}

	return tlsConfig
}
