package tidl

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"

	// TODO(ts): look at replacing bitio

	"github.com/icza/bitio"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/meta"
)

const baseurl = "https://api.tidalhifi.com/v1/"
const clientVersion = "1.9.1"
const token = "kgsOOmYk3zShYrNP"

const (
	AQ_LOSSLESS int = iota
	AQ_HI_RES
)

var cookieJar, _ = cookiejar.New(nil)
var c = &http.Client{
	Jar: cookieJar,
}

type TidalError struct {
	Status      int
	SubStatus   int
	UserMessage string
}

// Tidal api struct
type Tidal struct {
	albumMap    map[string]Album
	SessionID   string      `json:"sessionID"`
	CountryCode string      `json:"countryCode"`
	UserID      json.Number `json:"userId"`
}

// Artist struct
type Artist struct {
	ID   json.Number `json:"id"`
	Name string      `json:"name"`
	Type string      `json:"type"`
}

// Album struct
type Album struct {
	Artists              []Artist    `json:"artists,omitempty"`
	Title                string      `json:"title"`
	ID                   json.Number `json:"id"`
	NumberOfTracks       json.Number `json:"numberOfTracks"`
	Explicit             bool        `json:"explicit,omitempty"`
	Copyright            string      `json:"copyright,omitempty"`
	AudioQuality         string      `json:"audioQuality"`
	ReleaseDate          string      `json:"releaseDate"`
	Duration             float64     `json:"duration"`
	PremiumStreamingOnly bool        `json:"premiumStreamingOnly"`
	Popularity           float64     `json:"popularity,omitempty"`
	Artist               Artist      `json:"artist"`
	Cover                string      `json:"cover"`
	artBody              []byte
}

// Track struct
type Track struct {
	Artists      []Artist    `json:"artists"`
	Artist       Artist      `json:"artist"`
	Album        Album       `json:"album"`
	Title        string      `json:"title"`
	ID           json.Number `json:"id"`
	Explicit     bool        `json:"explicit"`
	Copyright    string      `json:"copyright"`
	Popularity   int         `json:"popularity"`
	TrackNumber  json.Number `json:"trackNumber"`
	Duration     json.Number `json:"duration"`
	AudioQuality string      `json:"audioQuality"`
}

// Search struct
type Search struct {
	Items  []Album `json:"items"`
	Albums struct {
		Items []Album `json:"items"`
	} `json:"albums"`
	Artists struct {
		Items []Artist `json:"items"`
	} `json:"artists"`
	Tracks struct {
		Items []Track `json:"items"`
	} `json:"tracks"`
}

func (t *Tidal) get(dest string, query *url.Values, s interface{}) error {
	// log.Println(baseurl+dest+"?"+query.Encode(), t.SessionID)
	req, err := http.NewRequest("GET", baseurl+dest, nil)
	if err != nil {
		return err
	}
	req.Header.Add("X-Tidal-SessionID", t.SessionID)
	query.Add("countryCode", t.CountryCode)
	req.URL.RawQuery = query.Encode()
	res, err := c.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	return json.NewDecoder(res.Body).Decode(&s)
}

func (t *Tidal) CheckSession() (bool, error) {
	//if self.user is None or not self.user.id or not self.session_id:
	//return False
	var out interface{}
	err := t.get(fmt.Sprintf("users/%s/subscription", t.UserID), nil, &out)
	// fmt.Println(out)
	return true, err
}

func (t *Tidal) GetFavoriteAlbums() ([]string, error) {
	var out struct {
		Limit              int `json:"limit"`
		Offset             int `json:"offset"`
		TotalNumberOfItems int `json:"totalNumberOfItems"`
		Items              []struct {
			Created string `json:"created"`
			Item    struct {
				ID                   int         `json:"id"`
				Title                string      `json:"title"`
				Duration             int         `json:"duration"`
				StreamReady          bool        `json:"streamReady"`
				StreamStartDate      string      `json:"streamStartDate"`
				AllowStreaming       bool        `json:"allowStreaming"`
				PremiumStreamingOnly bool        `json:"premiumStreamingOnly"`
				NumberOfTracks       int         `json:"numberOfTracks"`
				NumberOfVideos       int         `json:"numberOfVideos"`
				NumberOfVolumes      int         `json:"numberOfVolumes"`
				ReleaseDate          string      `json:"releaseDate"`
				Copyright            string      `json:"copyright"`
				Type                 string      `json:"type"`
				Version              interface{} `json:"version"`
				URL                  string      `json:"url"`
				Cover                string      `json:"cover"`
				VideoCover           interface{} `json:"videoCover"`
				Explicit             bool        `json:"explicit"`
				Upc                  string      `json:"upc"`
				Popularity           int         `json:"popularity"`
				AudioQuality         string      `json:"audioQuality"`
				SurroundTypes        interface{} `json:"surroundTypes"`
				Artist               struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"artist"`
				Artists []struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"artists"`
			} `json:"item"`
		} `json:"items"`
	}

	err := t.get(fmt.Sprintf("users/%s/favorites/albums", t.UserID), &url.Values{
		"limit": {"500"},
	}, &out)
	var ids []string

	for _, id := range out.Items {
		ids = append(ids, strconv.Itoa(id.Item.ID))
	}

	return ids, err
}

// GetStreamURL func
func (t *Tidal) GetStreamURL(id, q string) (string, error) {
	var s struct {
		URL string `json:"url"`
	}
	err := t.get("tracks/"+id+"/streamUrl", &url.Values{
		"soundQuality": {q},
	}, &s)

	// fmt.Println(s.URL)

	return s.URL, err
}

func (t *Tidal) GetAlbum(id string) (Album, error) {
	var s Album

	if album, ok := t.albumMap[id]; ok {
		return album, nil
	}

	err := t.get("albums/"+id, &url.Values{}, &s)
	t.albumMap[id] = s

	if s.Duration == 0 {
		return s, errors.New("album unavailable")
	}

	return s, err
}

// GetAlbumTracks func
func (t *Tidal) GetAlbumTracks(id string) ([]Track, error) {
	var s struct {
		Items []Track `json:"items"`
	}
	return s.Items, t.get("albums/"+id+"/tracks", &url.Values{}, &s)
}

// GetPlaylistTracks func
func (t *Tidal) GetPlaylistTracks(id string) ([]Track, error) {
	var s struct {
		Items []Track `json:"items"`
	}
	return s.Items, t.get("playlists/"+id+"/tracks", &url.Values{}, &s)
}

// SearchTracks func
func (t *Tidal) SearchTracks(d string, l int) ([]Track, error) {
	var s Search
	var limit string

	if l > 0 {
		limit = strconv.Itoa(l)
	}

	return s.Tracks.Items, t.get("search", &url.Values{
		"query": {d},
		"types": {"TRACKS"},
		"limit": {limit},
	}, &s)
}

// SearchAlbums func
func (t *Tidal) SearchAlbums(d string, l int) ([]Album, error) {
	var s Search
	var limit string

	if l > 0 {
		limit = strconv.Itoa(l)
	}

	err := t.get("search", &url.Values{
		"query": {d},
		"types": {"ALBUMS"},
		"limit": {limit},
	}, &s)

	if err != nil {
		return s.Albums.Items, err
	}

	for _, album := range s.Albums.Items {
		t.albumMap[album.ID.String()] = album
	}

	return s.Albums.Items, nil
}

// SearchArtists func
func (t *Tidal) SearchArtists(d string, l int) ([]Artist, error) {
	var s Search
	var limit string

	if l > 0 {
		limit = strconv.Itoa(l)
	}

	return s.Artists.Items, t.get("search", &url.Values{
		"query": {d},
		"types": {"ARTISTS"},
		"limit": {limit},
	}, &s)
}

func (t *Tidal) GetArtist(artist string) (Artist, error) {
	var s Artist
	err := t.get(fmt.Sprintf("artists/%s", artist), &url.Values{}, &s)
	return s, err
}

// GetArtistAlbums func
func (t *Tidal) GetArtistAlbums(artist string, l int) ([]Album, error) {
	var s Search
	var limit string

	if l > 0 {
		limit = strconv.Itoa(l)
	}

	err := t.get(fmt.Sprintf("artists/%s/albums", artist), &url.Values{
		"limit": {limit},
	}, &s)

	if err != nil {
		return s.Items, err
	}

	for _, album := range s.Items {
		t.albumMap[album.ID.String()] = album
	}

	return s.Items, nil
}

func (t *Tidal) GetArtistEP(artist string, l int) ([]Album, error) {
	var s Search
	var limit string

	if l > 0 {
		limit = strconv.Itoa(l)
	}

	err := t.get(fmt.Sprintf("artists/%s/albums", artist), &url.Values{
		"limit":  {limit},
		"filter": {"EPSANDSINGLES"},
	}, &s)

	if err != nil {
		return s.Items, err
	}

	for _, album := range s.Items {
		t.albumMap[album.ID.String()] = album
	}

	return s.Items, nil
}

func (al *Album) GetArt() ([]byte, error) {
	u := "https://resources.tidal.com/images/" + strings.Replace(al.Cover, "-", "/", -1) + "/1280x1280.jpg"
	res, err := http.Get(u)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}

func (t *Tidal) DownloadAlbum(al Album) error {
	tracks, err := t.GetAlbumTracks(al.ID.String())
	if err != nil {
		return err
	}

	if al.Duration == 0.0 {
		return errors.New("album unavailable")
	}

	dirs := clean(al.Artists[0].Name) + "/" + clean(al.Title)
	os.MkdirAll(dirs, os.ModePerm)

	metadata, err := json.MarshalIndent(al, "", "\t")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(dirs+"/meta.json", metadata, 0777)
	if err != nil {
		return err
	}

	body, err := al.GetArt()
	if err != nil {
		return err
	}

	al.artBody = body
	t.albumMap[al.ID.String()] = al

	err = ioutil.WriteFile(dirs+"/album.jpg", body, 0777)
	if err != nil {
		return err
	}

	for i, track := range tracks {
		fmt.Printf("\t [%v/%v] %v\n", i+1, len(tracks), track.Title)
		if err := t.DownloadTrack(track); err != nil {
			return err
		}
	}

	return nil
}

func (t *Tidal) DownloadTrack(tr Track) error {
	// TODO(ts): improve ID3
	al := t.albumMap[tr.Album.ID.String()]
	tr.Album = al

	dirs := clean(al.Artist.Name) + "/" + clean(al.Title)
	os.MkdirAll(dirs, os.ModePerm)

	u, err := t.GetStreamURL(tr.ID.String(), "LOSSLESS")
	if err != nil {
		return err
	}

	if u != "" {
		path := dirs + "/" + clean(tr.Artist.Name) + " - " + clean(tr.Title)

		_, err := os.Stat("./" + path + ".flac")
		if !os.IsNotExist(err) {
			return nil
		}

		f, err := os.Create(path)
		if err != nil {
			return err
		}

		res, err := http.Get(u)
		if err != nil {
			return err
		}

		io.Copy(f, res.Body)
		res.Body.Close()
		f.Close()

		err = enc(dirs, tr)
		if err != nil {
			return err
		}
		os.Remove(path)
	}

	return nil
}

// helper function to generate a uuid
func uuid() string {
	b := make([]byte, 16)
	rand.Read(b[:])
	b[8] = (b[8] | 0x40) & 0x7F
	b[6] = (b[6] & 0xF) | (4 << 4)
	return fmt.Sprintf("%x", b)
}

// New func
func New(user, pass string) (*Tidal, error) {
	query := url.Values{
		"username":        {user},
		"password":        {pass},
		"token":           {token},
		"clientUniqueKey": {uuid()},
		"clientVersion":   {clientVersion},
	}
	res, err := http.PostForm(baseurl+"login/username", query)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected error code from tidal: %d", res.StatusCode)
	}

	defer res.Body.Close()
	var t Tidal
	t.albumMap = make(map[string]Album)
	return &t, json.NewDecoder(res.Body).Decode(&t)
}

func clean(s string) string {
	return strings.Replace(s, "/", "\u2215", -1)
}

func enc(src string, tr Track) error {
	// https://wiki.hydrogenaud.io/index.php?title=Tag_Mapping#Titles
	// Decode FLAC file.
	path := src + "/" + clean(tr.Artist.Name) + " - " + clean(tr.Title)
	stream, err := flac.ParseFile(path)
	if err != nil {
		return err
	}

	// https://xiph.org/flac/format.html#metadata_block_picture
	MIMETYPE := "image/jpeg"
	pictureData := &bytes.Buffer{}
	w := bitio.NewWriter(pictureData)
	w.WriteBits(uint64(3), 32)                     // picture type (3)
	w.WriteBits(uint64(len(MIMETYPE)), 32)         // length of "image/jpeg"
	w.Write([]byte(MIMETYPE))                      // "image/jpeg"
	w.WriteBits(uint64(0), 32)                     // description length (0)
	w.Write([]byte{})                              // description
	w.WriteBits(uint64(1280), 32)                  // width (1280)
	w.WriteBits(uint64(1280), 32)                  // height (1280)
	w.WriteBits(uint64(24), 32)                    // colour depth (24)
	w.WriteBits(uint64(0), 32)                     // is pal? (0)
	w.WriteBits(uint64(len(tr.Album.artBody)), 32) // length of content
	w.Write(tr.Album.artBody)                      // actual content
	w.Close()

	encodedPictureData := base64.StdEncoding.EncodeToString(pictureData.Bytes())
	_ = encodedPictureData

	foundComments := false

	comments := [][2]string{}
	comments = append(comments, [2]string{"TITLE", tr.Title})
	comments = append(comments, [2]string{"ALBUM", tr.Album.Title})
	comments = append(comments, [2]string{"TRACKNUMBER", tr.TrackNumber.String()})
	comments = append(comments, [2]string{"TRACKTOTAL", tr.Album.NumberOfTracks.String()})
	comments = append(comments, [2]string{"ARTIST", tr.Artist.Name})
	comments = append(comments, [2]string{"ALBUMARTIST", tr.Album.Artist.Name})
	comments = append(comments, [2]string{"COPYRIGHT", tr.Copyright})
	comments = append(comments, [2]string{"METADATA_BLOCK_PICTURE", encodedPictureData})

	// Add custom vorbis comment.
	for _, block := range stream.Blocks {
		if comment, ok := block.Body.(*meta.VorbisComment); ok {
			foundComments = true
			comment.Tags = append(comment.Tags, comments...)
		}
	}

	if foundComments == false {
		block := new(meta.Block)
		block.IsLast = true
		block.Type = meta.Type(4)
		block.Length = 0

		comment := new(meta.VorbisComment)
		block.Body = comment
		comment.Vendor = "Lavf57.71.100"
		comment.Tags = append(comment.Tags, comments...)

		stream.Blocks = append(stream.Blocks, block)
	}

	// Encode FLAC file.
	f, err := os.Create(path + ".flac")
	if err != nil {
		return err
	}
	err = flac.Encode(f, stream)
	f.Close()
	stream.Close()
	return err
}
