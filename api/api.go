package api

import (
	"encoding/json"
	"github.com/nnanhthu/signal/transports"
)

//API plug-in structure
type API struct {
	caller transports.Caller
}

//NewAPI plug-in initialization
func NewAPI(caller transports.Caller) *API {
	return &API{caller}
}

func (api *API) Call(method string, params interface{}) (interface{}, int, error) {
	return api.caller.Call(method, params)
}

func (api *API) setCallback(apiID string, method string, callback func(raw json.RawMessage)) error {
	return api.caller.SetCallback(apiID, method, callback)
}
