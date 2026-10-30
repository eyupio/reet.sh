package api

import (
	"net/http"
	"strconv"
	"time"
)

// yoloSettingKey is the account-wide Allow YOLO switch; absent or "0" is off.
const yoloSettingKey = "terminal_allow_yolo"

const (
	yoloSettingByKey = "terminal_allow_yolo_by"
	yoloSettingAtKey = "terminal_allow_yolo_at"
)

type yoloSettingView struct {
	Allowed   bool   `json:"allowed"`
	ChangedBy string `json:"changedBy,omitempty"`
	ChangedAt int64  `json:"changedAt,omitempty"`
}

func (s *server) yoloSetting() (yoloSettingView, error) {
	var out yoloSettingView
	v, err := s.st.GetSetting(yoloSettingKey)
	if err != nil {
		return out, err
	}
	out.Allowed = v == "1"
	out.ChangedBy, _ = s.st.GetSetting(yoloSettingByKey)
	at, _ := s.st.GetSetting(yoloSettingAtKey)
	out.ChangedAt, _ = strconv.ParseInt(at, 10, 64)
	return out, nil
}

// yoloSettingGet is readable by writers, because the launch bar needs the
// gate before it offers YOLO; only owners and admins may change it.
func (s *server) yoloSettingGet(w http.ResponseWriter, r *http.Request) {
	out, err := s.yoloSetting()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) yoloSettingPut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Allowed *bool `json:"allowed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Allowed == nil {
		writeErr(w, http.StatusBadRequest, "allowed is required")
		return
	}
	value := "0"
	if *in.Allowed {
		value = "1"
	}
	name := s.currentUser(r).Name
	at := time.Now().UnixMilli()
	for key, v := range map[string]string{
		yoloSettingKey:   value,
		yoloSettingByKey: name,
		yoloSettingAtKey: strconv.FormatInt(at, 10),
	} {
		if err := s.st.SetSetting(key, v); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	action := "yolo-disallowed"
	if *in.Allowed {
		action = "yolo-allowed"
	}
	s.eventRequest(r, action, "", map[string]any{"allowed": *in.Allowed})
	out, err := s.yoloSetting()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
