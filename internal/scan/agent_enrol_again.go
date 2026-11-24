package scan

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/store"
)

// enrolAgain gives an enrolled machine a new credential from an enrolment
// code and changes nothing else: the configuration is copied, only its id,
// secret and personal token are replaced, and it is written in one step, so
// a refused exchange leaves the old one as it was. The agent is restarted
// as set-server restarts it.
func enrolAgain(config, code, servicePrefix string, in io.Reader, out io.Writer, version string) error {
	if _, err := os.Stat(config); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("this machine has no enrolment at %s to renew; enrol it with reet agent enroll", config)
	}
	c, err := loadAgentConfig(config)
	if err != nil {
		return err
	}
	if c.ID == "" || c.Secret == "" {
		return errors.New("this installation is not enrolled with a controller; enrol it with reet agent enroll")
	}
	code, err = clientconfig.ResolveEnrolCode(code, c.Server, in, os.Stderr)
	if err != nil {
		return err
	}
	exchange := agentConfig{Server: c.Server, Token: code}
	var result struct {
		Agent  store.SyncAgent
		Secret string
	}
	if err = agentRequest(exchange, "/api/v2/sync/agents", map[string]string{"host": c.Host, "version": version, "commit": buildCommit}, &result, version); err != nil {
		var refused controllerError
		if errors.As(err, &refused) && refused.Status == 401 {
			return errors.New("nothing was changed: the enrolment code was not accepted (mistyped, used already or expired). Make a new one under Machines, Add a machine")
		}
		return fmt.Errorf("nothing was changed: %w", err)
	}
	renewed := c
	renewed.ID, renewed.Secret, renewed.Token = result.Agent.ID, result.Secret, ""
	if err = editAgentConfig(config, func(s *agentConfig) error {
		s.ID, s.Secret, s.Token = renewed.ID, renewed.Secret, renewed.Token
		return nil
	}); err != nil {
		return fmt.Errorf("the controller gave this machine a new credential, but it could not be saved (%w); enrol it again with a new code", err)
	}
	what := "new machine credential saved"
	if c.Token != "" {
		what += ", personal token removed from the enrolment"
	}
	onOff := map[bool]string{true: "on", false: "off"}[c.AllowApply]
	fmt.Fprintf(out, "Enrolled %s again with a code: %s; local settings kept (remote apply: %s). Revoke the old token under Account in the web.\n", renewed.Host, what, onOff)
	if saved, _ := clientconfig.SavedToken(); c.Token != "" && saved == c.Token {
		path, _ := clientconfig.TokenPath()
		fmt.Fprintf(out, "The same token is still saved in %s for other reet commands; after revoking it, remove it with reet configure --forget-token.\n", path)
	}
	if err = controlAgentFn("restart", config, servicePrefix, out); err != nil {
		return fmt.Errorf("the new credential was saved, but the agent could not be restarted (%w); run reet agent restart", err)
	}
	return nil
}
