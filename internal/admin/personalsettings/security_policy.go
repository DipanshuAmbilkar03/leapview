package personalsettings

import (
	"errors"
	"net/http"
)

var ErrSecurityManagementForbidden = errors.New("account security management requires a platform administrator")

func (h Handler) canManageSecurity(r *http.Request, principalID string) (bool, error) {
	if h.PlatformAdmin == nil {
		return false, nil
	}
	return h.PlatformAdmin(r.Context(), principalID)
}

func (h Handler) rejectSecurityCommand(w http.ResponseWriter, r *http.Request, principalID string, signals commandSignals) bool {
	if signals.Session.Action == "" && signals.AuthoringSession.Action == "" && signals.Token.Action == "" {
		return false
	}
	allowed, err := h.canManageSecurity(r, principalID)
	if err != nil {
		h.writeError(w, r, err, http.StatusInternalServerError)
		return true
	}
	if !allowed {
		h.writeError(w, r, ErrSecurityManagementForbidden, http.StatusForbidden)
		return true
	}
	return false
}

func (h Handler) loadSecurityState(r *http.Request, principalID, sessionID string, tokensActive bool) (Signal, error) {
	allowed, err := h.canManageSecurity(r, principalID)
	if err != nil {
		return Signal{}, err
	}
	if tokensActive && !allowed {
		return Signal{}, ErrSecurityManagementForbidden
	}
	state, err := h.Service.Load(r.Context(), principalID, sessionID, tokensActive)
	state.Security.CanManageSessions = &allowed
	state.Tokens.CanManageTokens = &allowed
	if !allowed {
		state.Security.Sessions = []SessionSignal{}
		state.Security.AuthoringSessions = []AuthoringSessionSignal{}
		state.Tokens.Items = []TokenSignal{}
		state.Tokens.Capabilities = []CapabilityOptionSignal{}
		state.Tokens.NewToken = nil
	}
	return state, err
}
