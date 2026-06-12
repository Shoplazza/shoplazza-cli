package client

import (
	"bytes"
	"encoding/json"
	"mime"
	"strings"
)

// unmarshalUnwrapped decodes JSON bytes into out, stripping the Shoplazza
// envelope {"code":"Success","data":{...}} when present.
func unmarshalUnwrapped(data []byte, out any) error {
	if first := firstNonSpace(data); first != '{' {
		return decodeUseNumber(data, out)
	}
	var env struct {
		Code string          `json:"code"`
		Data json.RawMessage `json:"data"`
		OK   *bool           `json:"ok"`
	}
	if err := json.Unmarshal(data, &env); err == nil && len(env.Data) > 0 {
		if env.Code == "Success" || (env.OK != nil && *env.OK) {
			return decodeUseNumber(env.Data, out)
		}
	}
	return decodeUseNumber(data, out)
}

// decodeUseNumber is json.Unmarshal but preserves numeric precision by
// keeping numbers as json.Number instead of folding to float64.
func decodeUseNumber(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(out)
}

func firstNonSpace(b []byte) byte {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return c
		}
	}
	return 0
}

// unwrapDataEnvelope strips the Shoplazza OpenAPI transport envelope.
func unwrapDataEnvelope(body any) any {
	result, _ := unwrapEnvelope(body)
	return result
}

func unwrapEnvelope(body any) (any, bool) {
	m, ok := body.(map[string]any)
	if !ok {
		return body, false
	}
	data, hasData := m["data"]
	if !hasData {
		return body, false
	}
	if code, _ := m["code"].(string); code == "Success" {
		return data, true
	}
	if okFlag, hasOK := m["ok"].(bool); hasOK && okFlag {
		return data, true
	}
	return body, false
}

func parseResponseBody(contentType string, body []byte) (any, error) {
	if len(body) == 0 {
		return nil, nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
	}
	if strings.Contains(mediaType, "json") {
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		var decoded any
		if err := dec.Decode(&decoded); err != nil {
			return nil, err
		}
		return decoded, nil
	}
	return string(body), nil
}
