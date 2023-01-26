package main

import (
    "fmt"
    "time"
    "regexp"
    "context"
    "strings"
    "net/http"
    "path/filepath"
    "encoding/json"

    "golang.org/x/oauth2"
    "github.com/spf13/viper"
    "github.com/gorilla/mux"
    "github.com/google/go-github/v49/github"
)

type Code struct {
    Contents        string      `"json:contents"`
}

type Language struct {
    Name            string      `"json:name"`
    Extension       string      `"json:extension"`
}

type LanguageResponse struct {
    Code            *Code       `"json:code"`
    Language        *Language   `"json:language"`
    RequestedAt     time.Time   `"json:requested_at"`
}

type LanguagesResponse struct {
    Languages       []*Language `"json:languages"`
    RequestedAt     time.Time   `"json:requested_at"`
}

// Stolen from: https://github.com/google/go-github/blob/838d2238a6da019b49b571e8d8ebc5a6b12f8844/github/github.go#L863
type ErrorResponse struct {
    Request         *http.Request
    StatusCode      int         `json:"status_code"`
    Message         string      `json:"message"`
}

func (r *ErrorResponse) Error() string {
	return fmt.Sprintf("%v %v: %d %v",
		r.Request.Method, r.Request.URL,
		r.StatusCode, r.Message)
}

var ctx context.Context

func home(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(struct { Message string } {
        Message: "Welcome to the Hello World API",
    })
}

func getLanguages(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    client := authorize(r.Header.Get("Authorization"))

    // Get the README object.
    readme, _, err := client.Repositories.GetReadme(
        ctx,
        viper.GetString("repository.user"),
        viper.GetString("repository.name"),
        nil,
    )

    if err != nil {
        w.WriteHeader(err.(*github.ErrorResponse).Response.StatusCode)
        json.NewEncoder(w).Encode(err.Error())
        return
    }

    // Get the README contents.
    s, err := readme.GetContent()

    if err != nil {
        w.WriteHeader(err.(*github.ErrorResponse).Response.StatusCode)
        json.NewEncoder(w).Encode(err.Error())
        return
    }

    languages := findLanguages(s)

    res := &LanguagesResponse{
        Languages: languages,
        RequestedAt: time.Now(),
    }

    json.NewEncoder(w).Encode(res)
}

func getLanguage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    client := authorize(r.Header.Get("Authorization"))
    l := strings.TrimPrefix(r.URL.Path, "/api/language/")

    initial := strings.ToLower(l[0:1])

    if !regexp.MustCompile("^[a-z]$").MatchString(initial) {
        initial = "#"
    }

    _, dir, _, err := client.Repositories.GetContents(
        ctx,
        viper.GetString("repository.user"),
        viper.GetString("repository.name"),
        initial,
        nil,
    )

    if err != nil {
        w.WriteHeader(err.(*github.ErrorResponse).Response.StatusCode)
        json.NewEncoder(w).Encode(err.Error())
        return
    }

    language, err := findLanguage(dir, l)

    if err != nil {
        err.(*ErrorResponse).Request = r
        w.WriteHeader(err.(*ErrorResponse).StatusCode)
        json.NewEncoder(w).Encode(err.Error())
        return
    }

    file, _, _, err := client.Repositories.GetContents(
        ctx,
        viper.GetString("repository.user"),
        viper.GetString("repository.name"),
        initial + "/" + language.Name + language.Extension,
        nil,
    )

    if err != nil {
        w.WriteHeader(err.(*github.ErrorResponse).Response.StatusCode)
        json.NewEncoder(w).Encode(err.Error())
        return
    }

    s, err := file.GetContent()

    if err != nil {
        w.WriteHeader(err.(*github.ErrorResponse).Response.StatusCode)
        json.NewEncoder(w).Encode(err.Error())
        return
    }

    code := &Code{
        Contents: s,
    }

    res := &LanguageResponse{
        Code: code,
        Language: language,
        RequestedAt: time.Now(),
    }

    json.NewEncoder(w).Encode(res)
}

func main() {
    router := mux.NewRouter()
    ctx = context.Background()

    loadConfigs([]string {
        "env",
    })

    router.HandleFunc("/api", home).Methods(http.MethodGet)
    router.HandleFunc("/api/languages", getLanguages).Methods(http.MethodGet)
    router.HandleFunc("/api/language/{language}", getLanguage).Methods(http.MethodGet)

    http.ListenAndServe(
        ":" + viper.GetString("server.port"),
        router,
    )
}

// --- HELPERS ---

func authorize(s string) *github.Client {
    if s == "" {
        return github.NewClient(nil)
    }

    t := strings.Replace(s, "Bearer", "", 1)
    ts := oauth2.StaticTokenSource(
        &oauth2.Token{AccessToken: t},
    )
    tc := oauth2.NewClient(ctx, ts)

    return github.NewClient(tc)
}

func findLanguage(rcs []*github.RepositoryContent, l string) (*Language, error) {
    for _, rc := range rcs {
        if isLanguage(rc, l) {
            name := rc.GetName()
            ext := filepath.Ext(name)
            n := strings.TrimSuffix(name, ext)

            return &Language{
                Name: n,
                Extension: ext,
            }, nil
        }
    }

    return nil, &ErrorResponse{
        Request: nil,
		StatusCode: http.StatusNotFound,
		Message: "Not Found",
	}
}

func findLanguages(s string) (languages []*Language) {
    re := regexp.MustCompile("\\* \\[(.+)\\]\\(.+\\)\n")

    // Find list of languages: "* [Language Name](lang.ext)"
    for _, m := range re.FindAllStringSubmatch(s, -1) {
        ext := strings.TrimSuffix(filepath.Ext(m[0]), ")\n")

        languages = append(languages, &Language{
            Name: m[1],
            Extension: ext,
        })
    }

    return
}

func isLanguage(rc *github.RepositoryContent, l string) bool {
    name := rc.GetName()
    ext := filepath.Ext(name)
    n := strings.TrimSuffix(name, ext)

    return strings.ToLower(l) == strings.ToLower(n)
}

func loadConfigs(configs []string) (err error) {
    viper.AddConfigPath("config")

    for _, s := range configs {
        viper.SetConfigName(s)

        if err = viper.MergeInConfig(); err != nil {
            return
        }
    }

    return
}
