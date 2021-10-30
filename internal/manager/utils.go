package manager

import (
	"fmt"
	"github.com/antigloss/go/logger"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}
func parseOperLightColor(color string) (int, error) {
	color2 := strings.Split(strings.ToLower(color), "x")
	value, err := strconv.ParseInt(color2[len(color2)-1], 16, 0)
	if err != nil {
		logger.Errorf("Color parsing for value %s: %s", color, err)
		return 0, err
	}
	return int(value), err
}

func plural(count int, singular string) (result string) {
	if (count == 1) || (count == 0) {
		result = strconv.Itoa(count) + " " + singular + " "
	} else {
		result = strconv.Itoa(count) + " " + singular + "s "
	}
	return
}

func secondsToHuman(input int) (result string) {
	years := math.Floor(float64(input) / 60 / 60 / 24 / 7 / 30 / 12)
	seconds := input % (60 * 60 * 24 * 7 * 30 * 12)
	months := math.Floor(float64(seconds) / 60 / 60 / 24 / 7 / 30)
	seconds = input % (60 * 60 * 24 * 7 * 30)
	weeks := math.Floor(float64(seconds) / 60 / 60 / 24 / 7)
	seconds = input % (60 * 60 * 24 * 7)
	days := math.Floor(float64(seconds) / 60 / 60 / 24)
	seconds = input % (60 * 60 * 24)
	hours := math.Floor(float64(seconds) / 60 / 60)
	seconds = input % (60 * 60)
	minutes := math.Floor(float64(seconds) / 60)
	seconds = input % 60

	if years > 0 {
		result = plural(int(years), "year") + plural(int(months), "month") + plural(int(weeks), "week") + plural(int(days), "day") + plural(int(hours), "hour") + plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else if months > 0 {
		result = plural(int(months), "month") + plural(int(weeks), "week") + plural(int(days), "day") + plural(int(hours), "hour") + plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else if weeks > 0 {
		result = plural(int(weeks), "week") + plural(int(days), "day") + plural(int(hours), "hour") + plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else if days > 0 {
		result = plural(int(days), "day") + plural(int(hours), "hour") + plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else if hours > 0 {
		result = plural(int(hours), "hour") + plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else if minutes > 0 {
		result = plural(int(minutes), "minute") + plural(int(seconds), "second")
	} else {
		result = plural(int(seconds), "second")
	}

	return
}

func ts() string {
	// https://stackoverflow.com/questions/33119748/convert-time-time-to-string?rq=1
	return time.Now().Format(time.RFC1123)
}

type LightColor string
type LightMode int64

const (
	Crazy LightMode = iota
	Solid
	NightMode
	Sunshine

	LightColorOff = LightColor("off")
)

func (m LightMode) String() string {
	s, _ := m.XlateVal()
	return s
}

func (m LightMode) XlateVal() (string, int) {
	switch m {
	case Crazy:
		return "crazy", 0
	case Solid:
		return "solid", 1
	case NightMode:
		return "night-mode", 2
	case Sunshine:
		return "sunshine", 1 // same as solid
	}
	return "unknown", 0
}

func LightModeVal(l string) (LightMode, error) {
	if len(l) < 2 {
		return Crazy, fmt.Errorf("Use 2 or more characters than %s", l)
	}
	switch strings.ToLower(l)[:2] {
	case "cr":
		return Crazy, nil
	case "so":
		return Solid, nil
	case "ni":
		return NightMode, nil
	case "su":
		return Sunshine, nil
	}
	return Crazy, fmt.Errorf("No matches found for %s", l)
}

func (c LightColor) Int() int {
	val2 := strings.Split(strings.ToLower(string(c)), "x")
	valBase := 10
	if len(val2) > 1 {
		valBase = 16
	}
	val3, err := strconv.ParseInt(val2[len(val2)-1], valBase, 32)
	// if parsing worked, that is the number we want!
	if err == nil {
		return int(val3)
	}
	// try converting string to known values
	// https://www.rapidtables.com/web/color/
	switch strings.ToLower(string(c)) {
	case "out":
		return 0
	case "none":
		return 0
	case "off":
		return 0
	case "black":
		return 0
	case "red":
		return 0xff0000
	case "green":
		return 0x00ff00
	case "blue":
		return 0x0000ff
	case "yellow":
		return 0xffff00
	case "cyan":
		return 0x00ffff
	case "magenta":
		return 0xff00ff
	case "purple":
		return 0x4b0082
	case "pink":
		return 0xff1493
	case "orange":
		return 0xff8c00
	case "brown":
		return 0x8b4513
	case "gold":
		return 0xd4Af37
	case "snow":
		return 0xfffafa
	case "azure":
		return 0xf0ffff
	case "white":
		return 0xffffff
	}
	// catch all: random
	red, green, blue := rand.Intn(256), rand.Intn(256), rand.Intn(256)
	return blue + green<<8 + red<<16
}
