package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (a *App) usersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		users, err := a.projects.ListUsers()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, users)
	case http.MethodPost:
		user, err := readUserProfile(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := a.projects.CreateUser(user)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) userHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if strings.Contains(id, "/") || !validID(id) {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		user, err := a.projects.GetUser(id)
		if err != nil {
			status := http.StatusInternalServerError
			if isNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, user)
	case http.MethodPut:
		user, err := readUserProfile(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := a.projects.UpdateUser(id, user)
		if err != nil {
			status := http.StatusBadRequest
			if isNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, saved)
	case http.MethodDelete:
		if err := a.projects.DeleteUser(id); err != nil {
			status := http.StatusBadRequest
			if isNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) classesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		classes, err := a.projects.ListClasses()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, classes)
	case http.MethodPost:
		class, err := readClassProfile(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := a.projects.CreateClass(class)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) classHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/classes/")
	if strings.Contains(id, "/") || !validID(id) {
		writeError(w, http.StatusBadRequest, "invalid class id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		class, err := a.projects.GetClass(id)
		if err != nil {
			status := http.StatusInternalServerError
			if isNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, class)
	case http.MethodPut:
		class, err := readClassProfile(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := a.projects.UpdateClass(id, class)
		if err != nil {
			status := http.StatusBadRequest
			if isNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, saved)
	case http.MethodDelete:
		if err := a.projects.DeleteClass(id); err != nil {
			status := http.StatusBadRequest
			if isNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func readUserProfile(reader io.Reader) (UserProfile, error) {
	var user UserProfile
	limited := io.LimitReader(reader, 2*1024*1024)
	if err := json.NewDecoder(limited).Decode(&user); err != nil {
		return UserProfile{}, err
	}
	return user, nil
}

func readClassProfile(reader io.Reader) (ClassProfile, error) {
	var class ClassProfile
	limited := io.LimitReader(reader, 2*1024*1024)
	if err := json.NewDecoder(limited).Decode(&class); err != nil {
		return ClassProfile{}, err
	}
	return class, nil
}
