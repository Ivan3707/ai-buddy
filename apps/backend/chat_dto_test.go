package main

type source struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type chatResponse struct {
	Answer  string   `json:"answer"`
	Sources []source `json:"sources"`
	Mode    string   `json:"mode"`
}
