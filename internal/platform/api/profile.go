package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/store"
	"github.com/NHAS/reverse_ssh/internal/platform/userkeys"
)

type profileResponse struct {
	ID                 uint     `json:"id"`
	Username           string   `json:"username"`
	Role               string   `json:"role"`
	RSSHUsername       string   `json:"rsshUsername"`
	CanCreateProjects  bool     `json:"canCreateProjects"`
	AllowedProjects    []string `json:"allowedProjects"`
	MustChangePassword bool     `json:"mustChangePassword"`
	SSHAuthorizedKeys  string   `json:"sshAuthorizedKeys"`
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, profilePayload(currentUser(r)))
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	current := currentUser(r)

	request := struct {
		SSHAuthorizedKeys *string `json:"sshAuthorizedKeys"`
		CurrentPassword   string  `json:"currentPassword"`
		NewPassword       string  `json:"newPassword"`
		ConfirmPassword   string  `json:"confirmPassword"`
	}{}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	patch := store.WebUserPatch{}
	changed := false

	if request.SSHAuthorizedKeys != nil {
		validatedKeys, err := userkeys.ValidateAuthorizedKeys(*request.SSHAuthorizedKeys)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		patch.SSHAuthorizedKeys = &validatedKeys
		changed = true
	}

	currentPassword := strings.TrimSpace(request.CurrentPassword)
	newPassword := request.NewPassword
	confirmPassword := request.ConfirmPassword
	if currentPassword != "" || newPassword != "" || confirmPassword != "" {
		if currentPassword == "" || newPassword == "" || confirmPassword == "" {
			writeError(w, http.StatusBadRequest, "current password, new password, and confirmation are required")
			return
		}
		if _, err := s.store.Authenticate(current.Username, currentPassword); err != nil {
			if errors.Is(err, store.ErrInvalidCredentials) {
				writeError(w, http.StatusUnauthorized, "current password is incorrect")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if newPassword != confirmPassword {
			writeError(w, http.StatusBadRequest, "new password and confirmation must match")
			return
		}
		if err := store.ValidatePasswordComplexity(newPassword); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		clearTemporary := false
		patch.Password = &newPassword
		patch.MustChangePassword = &clearTemporary
		changed = true
	}

	if !changed {
		writeError(w, http.StatusBadRequest, "no profile changes supplied")
		return
	}

	updated, err := s.store.UpdateManagedUser(current.Username, patch)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if patch.Password != nil {
		rotated, err := s.store.RotateUserSessionVersion(current.Username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		updated = rotated
		if err := s.auth.SetSessionCookie(w, updated.ID, updated.SessionVersion, 12*time.Hour, requestIsSecure(r)); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if patch.SSHAuthorizedKeys != nil {
		if err := userkeys.SyncUserAuthorizedKeys(s.cfg.DataDir, updated.Username, *patch.SSHAuthorizedKeys); err != nil {
			rollback := current.SSHAuthorizedKeys
			_, _ = s.store.UpdateManagedUser(current.Username, store.WebUserPatch{
				SSHAuthorizedKeys: &rollback,
			})
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.syncRuntimeAccessForUser(r.Context(), current, updated); err != nil {
			writeError(w, http.StatusBadGateway, "runtime access sync failed: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, profilePayload(updated))
}

func profilePayload(user store.WebUser) profileResponse {
	allowedProjects := user.AllowedProjectList()
	if allowedProjects == nil {
		allowedProjects = []string{}
	}

	return profileResponse{
		ID:                 user.ID,
		Username:           user.Username,
		Role:               user.Role,
		RSSHUsername:       user.RSSHUsername,
		CanCreateProjects:  store.UserCanCreateProjects(user),
		AllowedProjects:    allowedProjects,
		MustChangePassword: user.MustChangePassword,
		SSHAuthorizedKeys:  user.SSHAuthorizedKeys,
	}
}
