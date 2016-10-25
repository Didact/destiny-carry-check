package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"text/tabwriter"
	"time"
)

const TrialsOfOsiris = "14"

var client http.Client

var (
	system   = flag.String("platform", "", "the platform you play on")
	gamertag = flag.String("gamertag", "", "your gamertag")
	apiKey   = flag.String("apikey", os.Getenv("BNETAPI"), "bnet api key")
	count    = flag.Int("count", 1, "how many games to check (on each character)")
)

var cache map[string]*PlayerStats

type apiTransport struct {
	apiKey string
}

func (a *apiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("X-API-Key", a.apiKey)
	return http.DefaultTransport.RoundTrip(req)
}

type CarryCondition struct {
	Name string
	Func func([]*PlayerStats) bool
}

var (
	eloBased = &CarryCondition{"elo", func(ps []*PlayerStats) bool {
		if len(ps) <= 1 {
			return false
		}
		min := ps[0].ELO
		max := ps[0].ELO

		for _, p := range ps[1:] {
			if p.ELO < min {
				min = p.ELO
			}
			if p.ELO > max {
				max = p.ELO
			}
		}
		return (max - min) >= 500
	}}
	kdrBased = &CarryCondition{"k/d", func(ps []*PlayerStats) bool {
		if len(ps) <= 1 {
			return false
		}
		min := ps[0].KDR
		max := ps[0].KDR

		for _, p := range ps[1:] {
			if p.KDR < min {
				min = p.KDR
			}

			if p.KDR > max {
				max = p.KDR
			}
		}

		return (max - min) >= 1
	}}
	lhBased = &CarryCondition{"lh", func(ps []*PlayerStats) bool {
		under5 := false
		over20 := false

		for _, p := range ps {
			if p.Flawless <= 5 {
				under5 = true
			}
			if p.Flawless >= 20 {
				over20 = true
			}
		}
		return under5 && over20
	}}
)

type Activity struct {
	Period          time.Time
	ActivityDetails struct {
		ReferenceID              int
		InstanceID               string
		Mode                     int
		ActivityTypeHashOverride int
		IsPrivate                bool
	}

	Values struct {
		// whole bunch of other stuff here that I don't care about
		Completed struct {
			StatID string
			Basic  struct {
				Value        float64
				DisplayValue string
			}
		}
		Standing struct {
			StatID string
			Basic  struct {
				Value        float64
				DisplayValue string
			}
		}
	}
}

type ActivityHistoryResponse struct {
	Response struct {
		Data struct {
			Activities []Activity
		}
	}
	ErrorCode       int
	ThrottleSeconds int
	ErrorStatus     string
	Message         string
	MessageData     interface{}
}

func GetActivityHistory(console int, accountID string, characterID string, count int, page int, mode string) *ActivityHistoryResponse {
	baseURL := "https://www.bungie.net/Platform/Destiny/Stats/ActivityHistory/%d/%s/%s/?page=%d&count=%d&mode=%s"
	url := fmt.Sprintf(baseURL, console, accountID, characterID, page, count, mode)
	resp, err := client.Get(url)
	if err != nil {
		log.Println(err)
	}
	if resp.Body == nil {
		log.Println(errors.New("nil body"))
	}
	defer resp.Body.Close()
	r := &ActivityHistoryResponse{}
	err = json.NewDecoder(resp.Body).Decode(r)
	if err != nil {
		log.Println(err)
	}
	return r
}

type PGCRResponse struct {
	Response struct {
		Data struct {
			Period          time.Time
			ActivityDetails struct {
				ReferenceID              int
				InstanceID               string
				Mode                     int
				ActivityTypeHashOverride int
				IsPrivate                bool
			}
			// one for each player
			Entries []struct {
				Standing float64
				Score    map[string]interface{}
				Player   struct {
					DestinyUserInfo struct {
						IconPath       string
						MembershipType int
						MembershipID   string
						DisplayName    string
					}
					CharacterClass string
					CharacterLevel int
					LightLevel     int
				}
				CharacterID string
			}
			Teams []struct {
				TeamID   int
				Standing struct {
					Basic struct {
						Value        float64
						DisplayValue string
					}
				}
				Score struct {
					Basic struct {
						Value        float64
						DisplayValue string
					}
				}
			}
		}
	}
	ErrorCode       int
	ThrottleSeconds int
	ErrorStatus     string
	Message         string
	MessageData     interface{}
}

func GetPGCR(activityID string) *PGCRResponse {
	baseURL := "https://www.bungie.net/Platform/Destiny/Stats/PostGameCarnageReport/%s/"
	url := fmt.Sprintf(baseURL, activityID)
	resp, err := client.Get(url)
	if err != nil {
		log.Println(err)
	}
	if resp.Body == nil {
		log.Println(errors.New("nil body"))
	}
	defer resp.Body.Close()
	s := &PGCRResponse{}
	err = json.NewDecoder(resp.Body).Decode(s)
	if err != nil {
		log.Println(err)
	}
	return s
}

type GuardianGGResponse struct {
	StatusCode int
	Data       struct {
		MembershipID   string
		MembershipType int
		Name           string
		ClanName       string
		ClanTag        string
		Modes          map[string]struct {
			ELO         float64
			ELOSolo     float64
			GamesPlayed int
			TimePlayed  int
			Wins        int
			Kills       int
			Deaths      int
			Assists     int
		}
	}
}

func GetGuardianGGInfo(accountID string) *GuardianGGResponse {
	baseURL := "https://api.guardian.gg/v2/players/%s"
	url := fmt.Sprintf(baseURL, accountID)
	resp, err := http.Get(url)
	if err != nil {
		log.Println(err)
	}
	if resp.Body == nil {
		log.Println(errors.New("nil body"))
	}
	defer resp.Body.Close()
	s := &GuardianGGResponse{}
	err = json.NewDecoder(resp.Body).Decode(s)
	if err != nil {
		log.Println(err)
	}
	return s
}

type DTRResponse struct {
	MembershipID   string
	MembershipType string
	DisplayName    string
	Kills          string
	Deaths         string
	MatchCount     string `json:"match_count"`
	KillsY2        string `json:"kills_y2"`
	DeathsY2       string `json:"deaths_y2"`
	Flawless       struct {
		Years map[string]struct {
			Count      int
			Characters map[string]struct {
				Count int
			}
		}
	} `json:"-"`
	ThisWeek []struct {
		Matches string
		Losses  string
		Kills   string
		Deaths  string
	}
}

func GetDTRInfo(accountID string) *DTRResponse {
	baseURL := "https://api.destinytrialsreport.com/player/%s"
	url := fmt.Sprintf(baseURL, accountID)
	resp, err := client.Get(url)
	if err != nil {
		log.Println(err)
	}
	if resp.Body == nil {
		log.Println(errors.New("nil body"))
	}
	defer resp.Body.Close()
	b := &bytes.Buffer{}

	tee := io.TeeReader(resp.Body, b)

	type adapter struct {
		DTRResponse
		Flawless json.RawMessage
	}

	a := []*adapter{&adapter{DTRResponse{}, json.RawMessage{}}}
	err = json.NewDecoder(tee).Decode(&a)
	if err != nil {
		log.Println(err)
		log.Println(b.String())
	}
	if bytes.Compare([]byte(a[0].Flawless), []byte("[]")) != 0 {
		json.Unmarshal(a[0].Flawless, &a[0].DTRResponse.Flawless)
	}
	return &(a[0].DTRResponse)
}

func GetTotalTrialsGames(accountID string) int {
	baseURL := "https://www.bungie.net/Platform/Destiny/Stats/2/%s/%s/?modes=TrialsOfOsiris"
	characters := GetCharacterIDsForAccount(accountID, "2")
	total := 0
	for _, c := range characters {
		url := fmt.Sprintf(baseURL, accountID, c)
		resp, err := client.Get(url)
		if err != nil {
			log.Println(err)
		}
		if resp.Body == nil {
			log.Println(errors.New("nil body"))
		}
		defer resp.Body.Close()

		var s struct {
			Response struct {
				TrialsOfOsiris struct {
					AllTime struct {
						ActivitiesEntered struct {
							StatID string
							Basic  struct {
								Value        float64
								DisplayValue string
							}
						}
					}
				}
			}
		}

		err = json.NewDecoder(resp.Body).Decode(&s)
		if err != nil {
			log.Println(err)
		}
		total += int(s.Response.TrialsOfOsiris.AllTime.ActivitiesEntered.Basic.Value)
	}
	return total
}

func GetAccountIDForGamertag(gamertag, platform string) string {
	baseURL := "https://www.bungie.net/Platform/Destiny/SearchDestinyPlayer/%s/%s/"
	url := fmt.Sprintf(baseURL, platform, gamertag)
	resp, err := client.Get(url)
	if err != nil {
		log.Println(err)
	}
	if resp.Body == nil {
		log.Println(errors.New("nil body"))
	}
	defer resp.Body.Close()
	var s struct {
		Response []struct {
			MembershipType int
			MembershipID   string
			DisplayName    string
		}
		ErrorCode   int
		ErrorStatus string
	}

	type w struct {
	}

	err = json.NewDecoder(resp.Body).Decode(&s)
	if err != nil {
		log.Println(err)
	}
	return s.Response[0].MembershipID
}

func GetCharacterIDsForAccount(accountID string, platform string) []string {
	baseURL := "https://www.bungie.net/Platform/Destiny/%s/Account/%s/Summary/"
	url := fmt.Sprintf(baseURL, platform, accountID)
	resp, err := client.Get(url)
	if err != nil {
		log.Println(err)
	}
	if resp.Body == nil {
		log.Println(errors.New("nil body"))
	}
	defer resp.Body.Close()

	var s struct {
		Response struct {
			Data struct {
				Characters []struct {
					CharacterBase struct {
						CharacterID string
					}
				}
			}
		}
		ErrorCode   int
		ErrorStatus string
	}
	err = json.NewDecoder(resp.Body).Decode(&s)
	if err != nil {
		log.Println(err)
	}
	characters := make([]string, len(s.Response.Data.Characters))
	for i, c := range s.Response.Data.Characters {
		characters[i] = c.CharacterBase.CharacterID
	}
	return characters
}

type PlayerStats struct {
	Name     string
	ELO, KDR float64
	Flawless int
}

func (p *PlayerStats) String() string {
	return fmt.Sprintf("%s\telo: %.f,\tkdr: %.2f,\tflawless: %d", p.Name, p.ELO, p.KDR, p.Flawless)
}

func GetStatsForPlayer(accountID string) *PlayerStats {
	if cached, ok := cache[accountID]; ok {
		return cached
	}
	g := GetGuardianGGInfo(accountID)
	name := g.Data.Name
	elo := g.Data.Modes[TrialsOfOsiris].ELO
	kdr := float64(g.Data.Modes[TrialsOfOsiris].Kills) / float64(g.Data.Modes[TrialsOfOsiris].Deaths)
	dtr := GetDTRInfo(accountID)
	flawless := dtr.Flawless.Years["1"].Count + dtr.Flawless.Years["2"].Count + dtr.Flawless.Years["3"].Count
	ps := &PlayerStats{name, elo, kdr, flawless}
	cache[accountID] = ps
	return ps
}

func GetTrialsGamesForGamertag(gamertag string, count int) []*Activity {
	accountID := GetAccountIDForGamertag(gamertag, "2")
	characterIDs := GetCharacterIDsForAccount(accountID, "2")

	var as []*Activity

	for _, characterID := range characterIDs {
		r := GetActivityHistory(2, accountID, characterID, count, 0, "TrialsOfOsiris")
		//TODO: less allocations
		for i := range r.Response.Data.Activities {
			as = append(as, &r.Response.Data.Activities[i])
		}
	}
	return as
}

func init() {
	flag.Parse()
	cache = make(map[string]*PlayerStats)
	client = http.Client{Transport: &apiTransport{apiKey: *apiKey}}
}

func main() {

	w := tabwriter.NewWriter(os.Stdout, 4, 8, 1, ' ', 0)

	carryChecks := []*CarryCondition{eloBased, kdrBased, lhBased}

	totalCarries := 0
	totalGames := 0

	as := GetTrialsGamesForGamertag(*gamertag, *count)
	totalGames := len(as)

	for _, a := range as {
		myStanding := a.Values.Standing.Basic.Value
		pgcr := GetPGCR(a.ActivityDetails.InstanceID)
		players := pgcr.Response.Data.Entries
		var stats []*PlayerStats
		for _, player := range players {
			// only check people on the other team
			if player.Standing != myStanding {
				stat := GetStatsForPlayer(player.Player.DestinyUserInfo.MembershipID)
				stats = append(stats, stat)
				fmt.Fprintf(w, "%s\n", stat)
			}
		}
		for _, condition := range carryChecks {
			any := false
			if condition.Func(stats) {
				any = true
				fmt.Fprintf(w, "maybe a carry based on %s\n", condition.Name)
			}
			if any {
				totalCarries += 1
			}
		}
		fmt.Fprintln(w, "---")

	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "total games:\t%d\n", totalGames)
	fmt.Fprintf(w, "total potential carries:\t%d\n", totalCarries)
	w.Flush()
}
