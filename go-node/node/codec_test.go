package node

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_DecodeBody(t *testing.T) {

	type testBody struct {
		MsgType string `json:"type"`
		Num     int    `json:"number"`
		Text    string `json:"text"`
	}

	tests := []struct {
		label        string
		input        []byte
		expectedSrc  string
		expectedDst  string
		expectedBody any
	}{
		{
			label:       "test body",
			input:       []byte(`{"src":"test","dest":"test","body":{"type":"test","number":1,"text":"text"}}`),
			expectedSrc: "test",
			expectedDst: "test",
			expectedBody: testBody{
				MsgType: "test",
				Num:     1,
				Text:    "text",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			var msg Message
			err := json.Unmarshal(tc.input, &msg)
			assert.Nil(t, err)
			actualBody, err := DecodeBody[testBody](&msg)
			assert.Nil(t, err)
			assert.Equal(t, tc.expectedSrc, msg.Src)
			assert.Equal(t, tc.expectedDst, msg.Dest)
			assert.Equal(t, tc.expectedBody, actualBody)
		})
	}

}
