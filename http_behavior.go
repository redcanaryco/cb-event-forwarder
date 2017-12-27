package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"text/template"
)

/* This is the HTTP implementation of the OutputHandler interface defined in main.go */
type HttpBehavior struct {
	dest    string
	headers map[string]string

	client *http.Client

	httpPostTemplate        *template.Template
	firstEventTemplate      *template.Template
	subsequentEventTemplate *template.Template
}

type HttpStatistics struct {
	Destination string `json:"destination"`
}

/* Construct the HttpBehavior object */
func (this *HttpBehavior) Initialize(dest string) error {
	this.httpPostTemplate = config.HttpPostTemplate
	this.firstEventTemplate = template.Must(template.New("first_event").Parse(`{{.}}`))
	this.subsequentEventTemplate = template.Must(template.New("subsequent_event").Parse("\n, {{.}}"))

	this.headers = make(map[string]string)

	this.dest = dest

	/* add authorization token, if applicable */
	if config.HttpAuthorizationToken != nil {
		this.headers["Authorization"] = *config.HttpAuthorizationToken
	}

	this.headers["Content-Type"] = *config.HttpContentType

	transport := &http.Transport{
		TLSClientConfig: config.TLSConfig,
	}
	this.client = &http.Client{Transport: transport}

	return nil
}

func (this *HttpBehavior) String() string {
	return "HTTP POST " + this.Key()
}

func (this *HttpBehavior) Statistics() interface{} {
	return HttpStatistics{
		Destination: this.dest,
	}
}

func (this *HttpBehavior) Key() string {
	return this.dest
}

/* This function does a POST of the given event to this.dest. UploadBehavior is called from within its own
   goroutine so we can do some expensive work here. */
func (this *HttpBehavior) Upload(fileName string, fp *os.File) UploadStatus {
	var err error = nil
	var uploadData UploadData

	/* Initialize the POST */
	reader, writer := io.Pipe()

	uploadData.FileName = fileName
	fileInfo, err := fp.Stat()
	if err == nil {
		uploadData.FileSize = fileInfo.Size()
	}
	uploadData.Events = make(chan UploadEvent)

	request, err := http.NewRequest("POST", this.dest, reader)

	go func() {
		defer writer.Close()

		// spawn goroutine to read from the file
		go convertFileIntoTemplate(fp, uploadData.Events, this.firstEventTemplate, this.subsequentEventTemplate)

		this.httpPostTemplate.Execute(writer, uploadData)
	}()

	/* Set the header values of the post */
	for key, value := range this.headers {
		request.Header.Set(key, value)
	}

	/* Execute the POST */
	resp, err := this.client.Do(request)
	if err != nil {
		return UploadStatus{fileName: fileName, result: err}
	}
	defer resp.Body.Close()

	/* Some sort of issue with the POST */
	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		errorData := resp.Status + "\n" + string(body)

		return UploadStatus{fileName: fileName,
			result: fmt.Errorf("HTTP request failed: Error code %s", errorData), status: resp.StatusCode}
	}
	return UploadStatus{fileName: fileName, result: err, status: 200}
}
