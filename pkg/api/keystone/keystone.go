package keystone

import (
	"time"

	"errors"
	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/middleware"
	m "github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/setting"
)

const (
	SESS_TOKEN            = "keystone_token"
	SESS_TOKEN_EXPIRATION = "keystone_expiration"
	SESS_TOKEN_PROJECT    = "keystone_project"
	TOKEN_BUFFER_TIME     = 5 // Tokens refresh if the token will expire in less than this many minutes
)

func getUserName(c *middleware.Context) (string, error) {
	var keystoneUserIdObj interface{}
	if keystoneUserIdObj = c.Session.Get(middleware.SESS_KEY_USERID); keystoneUserIdObj == nil {
		return "", errors.New("Session timed out trying to get keystone userId")
	}

	userQuery := m.GetUserByIdQuery{Id: keystoneUserIdObj.(int64)}
	if err := bus.Dispatch(&userQuery); err != nil {
		if err == m.ErrUserNotFound {
			return "", err
		}
	}
	return userQuery.Result.Login, nil
}

func getOrgName(c *middleware.Context) (string, error) {
	orgQuery := m.GetOrgByIdQuery{Id: c.OrgId}
	if err := bus.Dispatch(&orgQuery); err != nil {
		if err == m.ErrOrgNotFound {
			return "", err
		}
	}
	return orgQuery.Result.Name, nil
}

func getNewToken(c *middleware.Context) (string, error) {
	var username, project string
	var err error
	if username, err = getUserName(c); err != nil {
		return "", err
	}
	if project, err = getOrgName(c); err != nil {
		return "", err
	}

	var keystonePasswordObj interface{}
	if keystonePasswordObj = c.Session.Get(middleware.SESS_KEY_PASSWORD); keystonePasswordObj == nil {
		return "", errors.New("Session timed out trying to get keystone password")
	}

	auth := Auth_data{
		Username: username,
		Project:  project,
		Password: keystonePasswordObj.(string),
		Domain:   setting.KeystoneDefaultDomain,
		Server:   setting.KeystoneURL,
	}
	if err := AuthenticateScoped(&auth); err != nil {
		return "", err
	}

	c.Session.Set(SESS_TOKEN, auth.Token)
	c.Session.Set(SESS_TOKEN_EXPIRATION, auth.Expiration)
	c.Session.Set(SESS_TOKEN_PROJECT, project)
	// in keystone v3 the token is in the response header
	return auth.Token, nil
}

func validateCurrentToken(c *middleware.Context) (bool, error) {
	token := c.Session.Get(SESS_TOKEN)
	if token == nil {
		return false, nil
	}

	expiration_obj := c.Session.Get(SESS_TOKEN_EXPIRATION)
	if expiration_obj == nil || expiration_obj.(string) == "" {
		return false, nil
	}
	expiration, err := time.Parse(time.RFC3339, expiration_obj.(string))
	if err != nil {
		return false, err
	}

	now := time.Now()
	if now.After(expiration.Add(-TOKEN_BUFFER_TIME * time.Minute)) {
		return false, nil
	}

	project := c.Session.Get(SESS_TOKEN_PROJECT)
	org, err := getOrgName(c)
	if err != nil {
		return false, err
	}
	if org != project {
		return false, nil
	}

	return true, nil
}

func GetToken(c *middleware.Context) (string, error) {
	var token string
	var err error
	valid, err := validateCurrentToken(c)
	if valid {

		var sessionTokenObj interface{}
		if sessionTokenObj = c.Session.Get(SESS_TOKEN); sessionTokenObj == nil {
			return "", errors.New("Session timed out trying to get token")
		}
		return sessionTokenObj.(string), nil
	}
	if token, err = getNewToken(c); err != nil {
		return "", err
	}
	return token, nil
}
