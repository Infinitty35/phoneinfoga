package suppliers

import (
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNumverifySupplierSuccessCustomApiKey(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	number := "11115551212"
	apikey := "5ad5554ac240e4d3d31107941b35a5eb"

	expectedResult := &NumverifyValidateResponse{
		Valid:               true,
		Number:              "79516566591",
		LocalFormat:         "9516566591",
		InternationalFormat: "+79516566591",
		CountryPrefix:       "+7",
		CountryCode:         "RU",
		CountryName:         "Russian Federation",
		Location:            "Saint Petersburg and Leningrad Oblast",
		Carrier:             "OJSC St. Petersburg Telecom (OJSC Tele2-Saint-Petersburg)",
		LineType:            "mobile",
	}

	gock.New("https://api.apilayer.com").
		Get("/number_verification/validate").
		MatchHeader("Apikey", apikey).
		MatchParam("number", number).
		Reply(200).
		JSON(expectedResult)

	s := NewNumverifySupplier()

	got, err := s.Request().SetApiKey(apikey).SetApiType("apilayer").ValidateNumber(number)
	assert.Nil(t, err)

	assert.Equal(t, expectedResult, got)
}

func TestNumverifySupplierError(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	number := "11115551212"
	apikey := "5ad5554ac240e4d3d31107941b35a5eb"

	expectedResult := &NumverifyErrorResponse{
		Message: "You have exceeded your daily\\/monthly API rate limit. Please review and upgrade your subscription plan at https:\\/\\/apilayer.com\\/subscriptions to continue.",
	}

	gock.New("https://api.apilayer.com").
		Get("/number_verification/validate").
		MatchHeader("Apikey", apikey).
		MatchParam("number", number).
		Reply(429).
		JSON(expectedResult)

	s := NewNumverifySupplier()

	got, err := s.Request().SetApiKey(apikey).SetApiType("marketplace").ValidateNumber(number)
	assert.Nil(t, got)
	assert.Equal(t, errors.New("You have exceeded your daily\\/monthly API rate limit. Please review and upgrade your subscription plan at https:\\/\\/apilayer.com\\/subscriptions to continue."), err)
}

func TestNumverifySupplierHTTPError(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	number := "11115551212"

	_ = os.Setenv("NUMVERIFY_API_KEY", "5ad5554ac240e4d3d31107941b35a5eb")
	defer os.Clearenv()

	dummyError := errors.New("test")

	gock.New("https://api.apilayer.com").
		Get("/number_verification/validate").
		ReplyError(dummyError)

	s := NewNumverifySupplier()

	got, err := s.Request().SetApiType("apilayer").ValidateNumber(number)
	assert.Nil(t, got)
	assert.Equal(t, &url.Error{
		Op:  "Get",
		URL: "https://api.apilayer.com/number_verification/validate?number=11115551212",
		Err: dummyError,
	}, err)
}

func TestNumverifySupplierClassicSuccess(t *testing.T) {
	defer gock.Off()

	number := "14152007986"
	apikey := "classic-access-key"

	expectedResult := &NumverifyValidateResponse{
		Valid:               true,
		Number:              "14152007986",
		LocalFormat:         "4152007986",
		InternationalFormat: "+14152007986",
		CountryPrefix:       "+1",
		CountryCode:         "US",
		CountryName:         "United States of America",
		Location:            "Snfc Cntrl",
		Carrier:             "",
		LineType:            "landline",
	}

	for _, apiType := range []string{"", "classic", "CLASSIC"} {
		t.Run(apiType, func(t *testing.T) {
			defer gock.Off()

			gock.New("https://apilayer.net").
				Get("/api/validate").
				MatchParam("access_key", apikey).
				MatchParam("number", number).
				Reply(200).
				JSON(expectedResult)

			s := NewNumverifySupplier()
			got, err := s.Request().SetApiKey(apikey).SetApiType(apiType).ValidateNumber(number)
			assert.Nil(t, err)
			assert.Equal(t, expectedResult, got)
			assert.True(t, gock.IsDone())
		})
	}
}

func TestNumverifySupplierClassicError(t *testing.T) {
	defer gock.Off()

	number := "14152007986"
	apikey := "classic-access-key"

	gock.New("https://apilayer.net").
		Get("/api/validate").
		MatchParam("access_key", apikey).
		MatchParam("number", number).
		Reply(200).
		JSON(map[string]interface{}{
			"success": false,
			"error": map[string]interface{}{
				"code": 101,
				"type": "invalid_access_key",
				"info": "You have not supplied a valid API Access Key.",
			},
		})

	s := NewNumverifySupplier()
	got, err := s.Request().SetApiKey(apikey).SetApiType("classic").ValidateNumber(number)
	assert.Nil(t, got)
	assert.EqualError(t, err, "You have not supplied a valid API Access Key.")
	assert.NotContains(t, err.Error(), apikey)
	assert.True(t, gock.IsDone())
}

func TestNumverifySupplierClassicHTTPErrorRedactsKey(t *testing.T) {
	defer gock.Off()

	number := "14152007986"
	apikey := "super-secret-access-key"

	gock.New("https://apilayer.net").
		Get("/api/validate").
		ReplyError(errors.New("dial tcp: connection refused"))

	s := NewNumverifySupplier()
	got, err := s.Request().SetApiKey(apikey).SetApiType("classic").ValidateNumber(number)
	assert.Nil(t, got)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), apikey)
	assert.Contains(t, err.Error(), "REDACTED")
}

func TestNumverifySupplierRejectsUnknownAPIType(t *testing.T) {
	s := NewNumverifySupplier()
	got, err := s.Request().SetApiKey("secret").SetApiType("nope").ValidateNumber("14152007986")
	assert.Nil(t, got)
	assert.EqualError(t, err, "unsupported NUMVERIFY_API_TYPE: expected classic or apilayer")
}

func TestNumverifySupplierDoesNotLogAPIKey(t *testing.T) {
	defer gock.Off()

	apikey := "super-secret-access-key"
	number := "14152007986"

	gock.New("https://apilayer.net").
		Get("/api/validate").
		MatchParam("access_key", apikey).
		Reply(200).
		JSON(&NumverifyValidateResponse{Valid: true, Number: number})

	logger := logrus.StandardLogger()
	prevLevel := logger.GetLevel()
	logger.SetLevel(logrus.DebugLevel)
	savedHooks := logger.ReplaceHooks(make(logrus.LevelHooks))
	hook := &logCapture{}
	logger.AddHook(hook)
	t.Cleanup(func() {
		logger.SetLevel(prevLevel)
		logger.ReplaceHooks(savedHooks)
	})

	_, err := NewNumverifySupplier().Request().SetApiKey(apikey).SetApiType("classic").ValidateNumber(number)
	assert.NoError(t, err)
	assert.NotEmpty(t, hook.entries)
	for _, entry := range hook.entries {
		assert.NotContains(t, entry, apikey)
	}
}

type logCapture struct {
	entries []string
}

func (h *logCapture) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *logCapture) Fire(entry *logrus.Entry) error {
	serialized, err := entry.String()
	if err != nil {
		h.entries = append(h.entries, entry.Message)
		return nil
	}
	h.entries = append(h.entries, serialized)
	return nil
}
