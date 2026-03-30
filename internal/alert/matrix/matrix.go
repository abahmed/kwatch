package matrix

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
	"k8s.io/klog/v2"
)

type Matrix struct {
	homeServer     string
	accessToken    string
	internalRoomID string
	title          string
	text           string

	// reference for general app configuration
	appCfg *config.App
}

// NewMatrix returns new Matrix instance
func NewMatrix(config map[string]interface{}, appCfg *config.App) *Matrix {
	homeServer, ok := config["homeServer"].(string)
	if !ok || len(homeServer) == 0 {
		klog.InfoS("initializing matrix with empty homeServer")
		return nil
	}

	accessToken, ok := config["accessToken"].(string)
	if !ok || len(accessToken) == 0 {
		klog.InfoS("initializing matrix with empty accessToken")
		return nil
	}

	internalRoomID, ok := config["internalRoomId"].(string)
	if !ok || len(internalRoomID) == 0 {
		klog.InfoS("initializing matrix with empty internalRoomId")
		return nil
	}

	title, _ := config["title"].(string)
	text, _ := config["text"].(string)

	return &Matrix{
		homeServer:     homeServer,
		accessToken:    accessToken,
		internalRoomID: internalRoomID,
		title:          title,
		text:           text,
		appCfg:         appCfg,
	}
}

func (m *Matrix) Name() string {
	return "Matrix"
}

func (m *Matrix) SendMessage(msg string) error {
	return m.sendAPI(msg)
}

func (m *Matrix) SendEvent(e *event.Event) error {
	return m.sendAPI(e.FormatHtml(m.appCfg.ClusterName, m.text))
}

func (m *Matrix) sendAPI(formattedMsg string) error {
	plainMsg := stripHtmlRegex(formattedMsg)

	payload := struct {
		Msgtype       string `json:"msgtype"`
		Format        string `json:"format"`
		Body          string `json:"body"`
		FormattedBody string `json:"formatted_body"`
	}{
		Msgtype:       "m.text",
		Format:        "org.matrix.custom.html",
		Body:          plainMsg,
		FormattedBody: formattedMsg,
	}

	msgBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf(
			"%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s"+
				"?access_token=%s",
			m.homeServer,
			url.PathEscape(m.internalRoomID),
			k8s.RandomString(24),
			url.QueryEscape(m.accessToken),
		),
		bytes.NewBuffer(msgBytes),
	)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	client := k8s.GetDefaultClient()
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode > 399 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf(
			"call to matrix alert returned status code %d: %s",
			response.StatusCode,
			string(body))

	}

	return nil
}

// This method uses a regular expresion to remove HTML tags.
func stripHtmlRegex(s string) string {
	const regex = `<.*?>`
	r := regexp.MustCompile(regex)
	return r.ReplaceAllString(s, "")
}
