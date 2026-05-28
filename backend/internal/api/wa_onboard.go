package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// GET /api/wa/meta/app-config
// @Summary     Meta app config
// @Description Returns public Meta app config (app_id, es_config_id) needed by the frontend for Embedded Signup.
// @Tags        whatsapp
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  object{app_id=string,es_config_id=string,graph_version=string}
// @Router      /api/wa/meta/app-config [get]
func (s *Server) metaAppConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"app_id":        s.cfg.MetaAppID,
		"es_config_id":  s.cfg.MetaESConfigID,
		"graph_version": s.cfg.MetaGraphVersion,
	})
}

// POST /api/wa/onboard/exchange
// @Summary     Meta Embedded Signup exchange
// @Description Exchanges an Embedded Signup code for a long-lived access token, fetches the phone number ID, and stores both per-org. Requires Admin role.
// @Tags        whatsapp
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body  object{code=string,phone_number_id=string}  true  "OAuth code"
// @Success     200  {object}  object{phone_number_id=string,phone_display=string}
// @Failure     400  {object}  ErrorResponse
// @Failure     502  {object}  ErrorResponse
// @Router      /api/wa/onboard/exchange [post]
func (s *Server) metaOnboardExchange(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		Code          string `json:"code"`
		PhoneNumberID string `json:"phone_number_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		writeError(w, http.StatusBadRequest, "code required")
		return
	}

	version := s.cfg.MetaGraphVersion
	if version == "" {
		version = "v18.0"
	}

	shortToken, err := metaExchangeCode(version, s.cfg.MetaAppID, s.cfg.MetaAppSecret, body.Code)
	if err != nil {
		s.logger.Sugar().Errorw("meta onboard: code exchange failed", "err", err)
		writeError(w, http.StatusBadGateway, "token exchange failed: "+err.Error())
		return
	}

	longToken, err := metaLongLivedToken(version, s.cfg.MetaAppID, s.cfg.MetaAppSecret, shortToken)
	if err != nil {
		s.logger.Sugar().Errorw("meta onboard: long-lived exchange failed", "err", err)
		writeError(w, http.StatusBadGateway, "long-lived token failed: "+err.Error())
		return
	}

	phoneID := body.PhoneNumberID
	displayPhone := ""
	if phoneID == "" {
		phoneID, displayPhone, err = metaFetchPhoneNumberID(version, longToken)
		if err != nil {
			s.logger.Sugar().Errorw("meta onboard: fetch phone number failed", "err", err)
			writeError(w, http.StatusBadGateway, "fetch phone number failed: "+err.Error())
			return
		}
	}

	autoReply := true
	if _, err := s.db.UpsertWAChannelConfig(
		ac.OrgID, "meta", phoneID, longToken, "", "", "",
		map[string]string{"api_key": longToken, "phone_number": phoneID},
		&autoReply, nil,
	); err != nil {
		s.logger.Sugar().Errorw("meta onboard: upsert failed", "err", err)
		writeError(w, http.StatusInternalServerError, "save failed")
		return
	}
	if err := s.db.DeactivateOtherWAChannelConfigs(ac.OrgID, "meta"); err != nil {
		s.logger.Sugar().Warnw("meta onboard: deactivate others failed", "err", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"phone_number_id": phoneID,
		"phone_display":   displayPhone,
	})
}

func metaExchangeCode(version, appID, appSecret, code string) (string, error) {
	resp, err := http.PostForm(
		fmt.Sprintf("https://graph.facebook.com/%s/oauth/access_token", version),
		url.Values{
			"client_id":     {appID},
			"client_secret": {appSecret},
			"code":          {code},
		},
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return metaParseToken(resp.Body)
}

func metaLongLivedToken(version, appID, appSecret, shortToken string) (string, error) {
	u := fmt.Sprintf(
		"https://graph.facebook.com/%s/oauth/access_token?grant_type=fb_exchange_token&client_id=%s&client_secret=%s&fb_exchange_token=%s",
		version, url.QueryEscape(appID), url.QueryEscape(appSecret), url.QueryEscape(shortToken),
	)
	resp, err := http.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return metaParseToken(resp.Body)
}

func metaParseToken(r io.Reader) (string, error) {
	b, _ := io.ReadAll(r)
	var res struct {
		AccessToken string `json:"access_token"`
		Error       *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if res.Error != nil {
		return "", fmt.Errorf("%s", res.Error.Message)
	}
	if res.AccessToken == "" {
		return "", fmt.Errorf("empty token in response")
	}
	return res.AccessToken, nil
}

func metaFetchPhoneNumberID(version, token string) (id, display string, err error) {
	resp, err := http.Get(fmt.Sprintf(
		"https://graph.facebook.com/%s/me/whatsapp_business_accounts?access_token=%s&fields=id",
		version, url.QueryEscape(token),
	))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var wabaRes struct {
		Data  []struct{ ID string `json:"id"` } `json:"data"`
		Error *struct{ Message string `json:"message"` } `json:"error"`
	}
	if err = json.Unmarshal(b, &wabaRes); err != nil {
		return "", "", fmt.Errorf("parse waba: %w", err)
	}
	if wabaRes.Error != nil {
		return "", "", fmt.Errorf("%s", wabaRes.Error.Message)
	}
	if len(wabaRes.Data) == 0 {
		return "", "", fmt.Errorf("no WhatsApp Business Accounts found")
	}

	resp2, err := http.Get(fmt.Sprintf(
		"https://graph.facebook.com/%s/%s/phone_numbers?access_token=%s&fields=id,display_phone_number",
		version, wabaRes.Data[0].ID, url.QueryEscape(token),
	))
	if err != nil {
		return "", "", err
	}
	defer resp2.Body.Close()
	b2, _ := io.ReadAll(resp2.Body)
	var phoneRes struct {
		Data []struct {
			ID                 string `json:"id"`
			DisplayPhoneNumber string `json:"display_phone_number"`
		} `json:"data"`
		Error *struct{ Message string `json:"message"` } `json:"error"`
	}
	if err = json.Unmarshal(b2, &phoneRes); err != nil {
		return "", "", fmt.Errorf("parse phones: %w", err)
	}
	if phoneRes.Error != nil {
		return "", "", fmt.Errorf("%s", phoneRes.Error.Message)
	}
	if len(phoneRes.Data) == 0 {
		return "", "", fmt.Errorf("no phone numbers in WABA")
	}
	return phoneRes.Data[0].ID, phoneRes.Data[0].DisplayPhoneNumber, nil
}
