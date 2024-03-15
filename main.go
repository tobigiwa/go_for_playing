package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/xprop"
)

var (
	NamedWindows = make(map[xproto.Window]string)
	X            *xgbutil.XUtil
	windows      []xproto.Window
	res          []string
	name         string
	err          error
)

func main() {
	if X, err = xgbutil.NewConn(); err != nil {
		log.Fatal(err)
	}

	if windows, err = ewmh.ClientListStackingGet(X); err != nil {
		log.Fatalf("error getting windows stack list: %v", err)
	}

	for i := 0; i < len(windows); i++ {
		if name, err = getWindowClassName(X, windows[i]); err != nil {
			fmt.Printf("error getting window name for window:%v error: %v", windows[i], err)
			continue
		}

		fmt.Printf("Window: %d, Name: %s\n", windows[i], name)
		NamedWindows[windows[i]] = name
	}

	if res, err = getVisibleWindow(X.Conn(), X, windows[len(windows)-1], windows[1:len(windows)-1]); err != nil {
		fmt.Println()
		fmt.Println()
		if errors.Is(err, ErrActiveIsFullScreen) {
			fmt.Println(err)
			return
		}

		if errors.Is(err, ErrDoesNotCover) {
			fmt.Println(err.Error())
			fmt.Println(res)
			return
		}

		fmt.Println("\n\nERROR:", err)
		return
	}

	fmt.Printf("\n\nwindows that makes up 80%% of the screen are %v\n", res)
}

func getVisibleWindow(X *xgb.Conn, X11 *xgbutil.XUtil, activeWindow xproto.Window, otherWindows []xproto.Window) ([]string, error) {

	geom1, err := xproto.GetGeometry(X, xproto.Drawable(activeWindow)).Reply()
	if err != nil {
		return nil, err
	}

	if geom1.Root != X11.RootWin() {
		fmt.Printf("focus was not a root window, root was, %v:%v", geom1.Root, NamedWindows[geom1.Root])
		_, err := xproto.TranslateCoordinates(X, activeWindow, X11.RootWin(), geom1.X, geom1.Y).Reply()
		if err != nil {
			return nil, err
		}
		log.Fatal()
	}

	rootGeom, err := xproto.GetGeometry(X, xproto.Drawable(X11.RootWin())).Reply()
	if err != nil {
		return nil, err
	}

	activeArea := int(geom1.Width) * int(geom1.Height)
	rootArea := int(rootGeom.Width) * int(rootGeom.Height)

	// activeArea := clipToScreen(trans1, geom1, rootGeom)

	if cover := float32(activeArea) / float32(rootArea); cover >= 0.8 {
		fmt.Println("The active window covers more than 80% of the screen-->", cover*100)
		return []string{}, ErrActiveIsFullScreen
	}

	coverage := activeArea
	visibleWindows := []string{NamedWindows[activeWindow]}

	for i := len(otherWindows) - 1; i >= 0; i-- {

		otherWindow := otherWindows[i]

		attrs, err := xproto.GetWindowAttributes(X, otherWindow).Reply()
		if err != nil {
			return nil, err
		}

		if attrs.MapState != xproto.MapStateViewable {
			// The window is not mapped, so skip it.
			fmt.Printf("\nwindow:%v was not viewable\n\n", NamedWindows[otherWindow])
			continue
		}

		geom2, err := xproto.GetGeometry(X, xproto.Drawable(otherWindow)).Reply()
		if err != nil {
			return nil, err
		}

		if geom2.Root != X11.RootWin() {
			fmt.Printf("otherWindow was not a root window, root was, %v:%v", geom2.Root, NamedWindows[geom2.Root])
			_, err := xproto.TranslateCoordinates(X, otherWindow, X11.RootWin(), geom2.X, geom2.Y).Reply()
			if err != nil {
				return nil, err
			}
			log.Fatal()
		}

		area2 := int(geom2.Width) * int(geom2.Height)
		// area2 := clipToScreen(trans2, geom2, rootGeom)

		x_overlap := max(0, min(int(geom1.X)+int(geom1.Width), int(geom2.X)+int(geom2.Width))-max(int(geom1.X), int(geom2.X)))
		y_overlap := max(0, min(int(geom1.Y)+int(geom1.Height), int(geom2.X)+int(geom2.Height))-max(int(geom1.Y), int(geom2.Y)))

		overlapArea := x_overlap * y_overlap

		overlapRatio := float32(overlapArea) / float32(area2)

		fmt.Println("\n", overlapArea, area2, overlapRatio)
		switch {

		case overlapRatio >= 0.8:
			fmt.Printf("window:%v is 80%% covered by focus:%v, actual percentage cover is %v%%\n", NamedWindows[otherWindow], NamedWindows[activeWindow], overlapRatio*100)
			continue

		case overlapRatio <= 0.1:
			fmt.Printf("window:%v apppears to have little or no overlap with the focus,actual overlapRatio is %v\n", NamedWindows[otherWindow], overlapRatio)
			combinedArea := activeArea + area2
			
			var cover float32
			if cover = float32(combinedArea) / float32(rootArea); cover >= 0.80 {
				visibleWindows = append(visibleWindows, NamedWindows[otherWindow])
				fmt.Printf("window:%v with focus COVERS more than 80%% of the scree\n", NamedWindows[otherWindow])
				return visibleWindows, nil
			}

			visibleWindows = append(visibleWindows, NamedWindows[otherWindow])
			fmt.Printf("window:%v DOES NOT COVER 80%% of the screen with focus, cover was %v%% \n", NamedWindows[otherWindow], cover*100)
			continue

		default:
			fmt.Printf("overlap percentage of window:%v with focus:%v is %v%%\n", NamedWindows[otherWindow], NamedWindows[activeWindow], overlapRatio*100)
			coverage += area2 - overlapArea

			if cover := float64(coverage) / float64(rootArea); cover >= 0.80 {
				visibleWindows = append(visibleWindows, NamedWindows[otherWindow])
				fmt.Printf("\nThe combination of windows %v COVERS 80%% of the screen, actual percentage is - %v%%\n", visibleWindows, cover*100)
				return visibleWindows, nil
			}

			visibleWindows = append(visibleWindows, NamedWindows[otherWindow])
			fmt.Printf("window:%v was added to visible because it overlap is more than 10%% and less than 80%% but does not make 80%% coverage with full screen, visible windows are %v --->", NamedWindows[otherWindow], visibleWindows)
			continue
		}

	}

	return visibleWindows, ErrDoesNotCover
}

var (
	ErrDoesNotCover       = fmt.Errorf("none of the windows in the slice caused the total coverage to be more than 80%% of the screen")
	ErrActiveIsFullScreen = fmt.Errorf("active screen covers more than 80%% of the screen")
)

// func clipToScreen(trans *xproto.TranslateCoordinatesReply, geom *xproto.GetGeometryReply, rootGeom *xproto.GetGeometryReply) int {
// 	x := int(trans.DstX)
// 	y := int(trans.DstY)
// 	width := int(geom.Width)
// 	height := int(geom.Height)
// 	screenWidth := int(rootGeom.Width)
// 	screenHeight := int(rootGeom.Height)

// 	if x < 0 {
// 		width += x
// 		x = 0
// 	}
// 	if y < 0 {
// 		height += y
// 		y = 0
// 	}
// 	if x+width > screenWidth {
// 		width = screenWidth - x
// 	}
// 	if y+height > screenHeight {
// 		height = screenHeight - y
// 	}
// 	return max(0, width) * max(0, height) // Return the area
// }

func getWindowClassName(X *xgbutil.XUtil, win xproto.Window) (string, error) {

	wmClass, err1 := xprop.PropValStrs(xprop.GetProperty(X, win, "WM_CLASS"))
	if err1 == nil && (len(wmClass) == 2) {
		return wmClass[1], nil
	}
	return "", fmt.Errorf("error on resolving name for window %d: %v", win, err1)
}

// func getVisibleWindow(X *xgb.Conn, X11 *xgbutil.XUtil, activeWindow xproto.Window, otherWindows []xproto.Window) (int, error) {
// 	geom1, err := xproto.GetGeometry(X, xproto.Drawable(activeWindow)).Reply()
// 	if err != nil {
// 		return 0, err
// 	}

// 	rootGeom, err := xproto.GetGeometry(X, xproto.Drawable(X11.RootWin())).Reply()
// 	if err != nil {
// 		return 0, err
// 	}

// 	trans1, err := xproto.TranslateCoordinates(X, activeWindow, X11.RootWin(), geom1.X, geom1.Y).Reply()
// 	if err != nil {
// 		return 0, err
// 	}

// 	// activeArea := int(geom1.Width) * int(geom1.Height)
// 	activeArea := clipToScreen(trans1, geom1, rootGeom)
// 	rootArea := int(rootGeom.Width) * int(rootGeom.Height)

// 	coverage := activeArea
// 	visibleWindows := []string{NamedWindows[activeWindow]}

// 	for i := len(otherWindows) - 1; i >= 0; i-- {
// 		otherWindow := otherWindows[i]
// 		geom2, err := xproto.GetGeometry(X, xproto.Drawable(otherWindow)).Reply()
// 		if err != nil {
// 			return 0, err
// 		}

// 		trans2, err := xproto.TranslateCoordinates(X, otherWindow, X11.RootWin(), geom2.X, geom2.Y).Reply()
// 		if err != nil {
// 			return 0, err
// 		}

// 		// area2 := int(geom2.Width) * int(geom2.Height)
// 		area2 := clipToScreen(trans2, geom2, rootGeom)

// 		x_overlap := max(0, min(int(trans1.DstX)+int(geom1.Width), int(trans2.DstX)+int(geom2.Width))-max(int(trans1.DstX), int(trans2.DstX)))
// 		y_overlap := max(0, min(int(trans1.DstY)+int(geom1.Height), int(trans2.DstY)+int(geom2.Height))-max(int(trans1.DstY), int(trans2.DstY)))

// 		overlapArea := x_overlap * y_overlap

// 		overlapRatio := float64(overlapArea) / float64(area2)
// 		fmt.Println(float64(overlapArea), float64(area2))

// 		if overlapRatio < 0.97 {
// 			coverage += area2 - overlapArea
// 			visibleWindows = append(visibleWindows, NamedWindows[otherWindow])
// 		}

// 		fmt.Printf("Windows --> %v, last Overlap: %v, stage: %v%%\n", visibleWindows, overlapRatio, int(math.Round(float64(coverage)/float64(rootArea)*100)))

// 		if float64(coverage)/float64(rootArea) >= 0.80 {
// 			fmt.Printf("The combination of windows %v covers more than 80%% of the screen\n", visibleWindows)
// 			return 100, nil
// 		}
// 	}

// 	return 0, nil // None of the windows in the slice caused the total coverage to be more than 80% of the screen
// }

var (
// prevGeom *xproto.GetGeometryReply = geom1
// prevTrans *xproto.TranslateCoordinatesReply = trans1
// count = 0
)
