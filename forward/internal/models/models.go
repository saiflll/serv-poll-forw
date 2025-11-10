package models

type TempData struct {
	No   int      `json:"no"`
	Ts   string   `json:"ts"`
	Temp float64  `json:"temp"`
	RH   *float64 `json:"rh"`
}

type DoorData struct {
	DoorID int `json:"doorid"`
	Value  int `json:"value"`
}

type AreaData struct {
	CK   int        `json:"ck"`
	Area int        `json:"area"`
	Door []DoorData `json:"door"`
	Temp []TempData `json:"temp"`
}
