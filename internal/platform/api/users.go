package api

import (
	"crypto/rand"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/store"
	"github.com/NHAS/reverse_ssh/internal/platform/userkeys"
	"gorm.io/gorm"
)

type managedUserResponse struct {
	Username           string    `json:"username"`
	Role               string    `json:"role"`
	RSSHUsername       string    `json:"rsshUsername"`
	Enabled            bool      `json:"enabled"`
	MustChangePassword bool      `json:"mustChangePassword"`
	CanCreateProjects  bool      `json:"canCreateProjects"`
	AllowedProjects    []string  `json:"allowedProjects"`
	SSHKeyCount        int       `json:"sshKeyCount"`
	SSHAuthorizedKeys  string    `json:"sshAuthorizedKeys,omitempty"`
	GeneratedPassword  string    `json:"generatedPassword,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !canManageUsers(user) {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	users, err := s.store.ListWebUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]managedUserResponse, 0, len(users))
	for _, item := range users {
		if item.Role == "admin" {
			continue
		}
		items = append(items, managedUserPayload(item, false, ""))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !canManageUsers(user) {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	targetUsername := strings.TrimSpace(r.PathValue("username"))
	target, err := s.store.GetUserByUsername(targetUsername)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, managedUserPayload(target, true, ""))
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !canManageUsers(user) {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	request := struct {
		Username          string   `json:"username"`
		AllowedProjects   []string `json:"allowedProjects"`
		CanCreateProjects bool     `json:"canCreateProjects"`
		SSHAuthorizedKeys string   `json:"sshAuthorizedKeys"`
	}{}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	validatedKeys, err := userkeys.ValidateAuthorizedKeys(request.SSHAuthorizedKeys)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	password, err := generateRandomPassword(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	created, err := s.store.CreateManagedUser(request.Username, password, request.CanCreateProjects, request.AllowedProjects, validatedKeys)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrUserExists):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	if err := userkeys.SyncUserAuthorizedKeys(s.cfg.DataDir, created.Username, validatedKeys); err != nil {
		_ = s.store.DeleteManagedUser(created.Username)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.syncRuntimeAccessForUser(r.Context(), created); err != nil {
		_ = userkeys.RemoveUserAuthorizedKeys(s.cfg.DataDir, created.Username)
		_ = s.store.DeleteManagedUser(created.Username)
		writeError(w, http.StatusBadGateway, "runtime access sync failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, managedUserPayload(created, true, password))
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !canManageUsers(user) {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	targetUsername := strings.TrimSpace(r.PathValue("username"))
	current, err := s.store.GetUserByUsername(targetUsername)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	request := struct {
		AllowedProjects   *[]string `json:"allowedProjects"`
		CanCreateProjects *bool     `json:"canCreateProjects"`
		SSHAuthorizedKeys *string   `json:"sshAuthorizedKeys"`
		ResetPassword     bool      `json:"resetPassword"`
	}{}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	patch := store.WebUserPatch{
		AllowedProjects:   request.AllowedProjects,
		CanCreateProjects: request.CanCreateProjects,
	}

	var generatedPassword string
	if request.ResetPassword {
		password, err := generateRandomPassword(16)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		mustChangePassword := true
		patch.Password = &password
		patch.MustChangePassword = &mustChangePassword
		generatedPassword = password
	}

	if request.SSHAuthorizedKeys != nil {
		validatedKeys, err := userkeys.ValidateAuthorizedKeys(*request.SSHAuthorizedKeys)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		patch.SSHAuthorizedKeys = &validatedKeys
	}

	updated, err := s.store.UpdateManagedUser(targetUsername, patch)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if patch.Password != nil {
		rotated, err := s.store.RotateUserSessionVersion(targetUsername)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		updated = rotated
	}

	if patch.SSHAuthorizedKeys != nil {
		if err := userkeys.SyncUserAuthorizedKeys(s.cfg.DataDir, updated.Username, *patch.SSHAuthorizedKeys); err != nil {
			rollback := current.SSHAuthorizedKeys
			_, _ = s.store.UpdateManagedUser(targetUsername, store.WebUserPatch{
				SSHAuthorizedKeys: &rollback,
			})
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := s.syncRuntimeAccessForUser(r.Context(), current, updated); err != nil {
		writeError(w, http.StatusBadGateway, "runtime access sync failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, managedUserPayload(updated, true, generatedPassword))
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !canManageUsers(user) {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	targetUsername := strings.TrimSpace(r.PathValue("username"))
	if targetUsername == user.Username {
		writeError(w, http.StatusBadRequest, "cannot delete the current authenticated user")
		return
	}

	target, err := s.store.GetUserByUsername(targetUsername)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.store.DeleteManagedUser(targetUsername); err != nil {
		switch {
		case errors.Is(err, store.ErrCannotDeleteAdmin):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	if err := userkeys.RemoveUserAuthorizedKeys(s.cfg.DataDir, targetUsername); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.syncRuntimeAccessForUser(r.Context(), target); err != nil {
		writeError(w, http.StatusBadGateway, "runtime access sync failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":  true,
		"username": targetUsername,
	})
}

func managedUserPayload(user store.WebUser, includeKeys bool, generatedPassword string) managedUserResponse {
	allowedProjects := user.AllowedProjectList()
	if allowedProjects == nil {
		allowedProjects = []string{}
	}
	payload := managedUserResponse{
		Username:           user.Username,
		Role:               user.Role,
		RSSHUsername:       user.RSSHUsername,
		Enabled:            user.Enabled,
		MustChangePassword: user.MustChangePassword,
		CanCreateProjects:  store.UserCanCreateProjects(user),
		AllowedProjects:    allowedProjects,
		SSHKeyCount:        userkeys.KeyCount(user.SSHAuthorizedKeys),
		GeneratedPassword:  generatedPassword,
		CreatedAt:          user.CreatedAt,
		UpdatedAt:          user.UpdatedAt,
	}
	if includeKeys {
		payload.SSHAuthorizedKeys = user.SSHAuthorizedKeys
	}
	return payload
}

func generateRandomPassword(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	if length <= 0 {
		length = 16
	}

	out := make([]byte, length)
	max := big.NewInt(int64(len(alphabet)))
	for index := range out {
		value, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[index] = alphabet[value.Int64()]
	}

	return string(out), nil
}
