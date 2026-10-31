//go:build ignore

// Generates the figures of the level card: 16 figures in 8 stages each. Stage 1
// means nothing is done yet, stage 8 means the whole day is done.
// Run from the repository root: go run tools/genfigures/main.go
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	green  = "#4fb283"
	gold   = "#f2c14e"
	orange = "#f2994a"
	red    = "#f2705b"
	blue   = "#7fb8f0"
	purple = "#c79bf2"
	steel  = "#c3d0c9"
	dark   = "#2a2f2c"
	track  = "#4a514d"
	brown  = "#8a5a3c"
	clay   = "#b9683f"
	leaf   = "#3f9a6b"
	ink    = "#0e1110"
)

var f = fmt.Sprintf

func circle(cx, cy, r float64, fill string) string {
	return f(`<circle cx="%g" cy="%g" r="%g" fill="%s"/>`, cx, cy, r, fill)
}

func ring(cx, cy, r, w float64, stroke string) string {
	return f(`<circle cx="%g" cy="%g" r="%g" fill="none" stroke="%s" stroke-width="%g"/>`, cx, cy, r, stroke, w)
}

func rect(x, y, w, h, rx float64, fill string) string {
	return f(`<rect x="%g" y="%g" width="%g" height="%g" rx="%g" fill="%s"/>`, x, y, w, h, rx, fill)
}

func frame(x, y, w, h, rx, sw float64, stroke, fill string) string {
	return f(`<rect x="%g" y="%g" width="%g" height="%g" rx="%g" fill="%s" stroke="%s" stroke-width="%g"/>`, x, y, w, h, rx, fill, stroke, sw)
}

func path(d, fill string) string { return f(`<path d="%s" fill="%s"/>`, d, fill) }

func stroke(d, color string, w float64) string {
	return f(`<path d="%s" fill="none" stroke="%s" stroke-width="%g" stroke-linecap="round" stroke-linejoin="round"/>`, d, color, w)
}

func fade(opacity float64, body string) string {
	return f(`<g opacity="%g">%s</g>`, opacity, body)
}

func shadow(rx float64) string {
	return f(`<ellipse cx="32" cy="60" rx="%g" ry="2" fill="#000" opacity=".3"/>`, rx)
}

func sparkle(x, y, r float64, color string) string {
	return path(f("M%g %gQ%g %g %g %gQ%g %g %g %gQ%g %g %g %gQ%g %g %g %gZ",
		x, y-r, x+r*.15, y-r*.15, x+r, y, x+r*.15, y+r*.15, x, y+r, x-r*.15, y+r*.15, x-r, y, x-r*.15, y-r*.15, x, y-r), color)
}

func steam(x float64, n int, y float64) string {
	out := ""
	for i := range n {
		sx := x + float64(i)*6 - float64(n-1)*3
		out += stroke(f("M%g %gq-3 -4 0 -7q3 -4 0 -7", sx, y), "#8d9690", 1.8)
	}
	return out
}

func mix(a, b string, t float64) string {
	parse := func(s string) (r, g, bl int) {
		fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &bl)
		return
	}
	r1, g1, b1 := parse(a)
	r2, g2, b2 := parse(b)
	c := func(x, y int) int { return x + int(math.Round(float64(y-x)*t)) }
	return f("#%02x%02x%02x", c(r1, r2), c(g1, g2), c(b1, b2))
}

func plant(s int) string {
	out := shadow(14)
	stem := []float64{0, 3, 8, 13, 18, 23, 27, 30}[s-1]
	top := 44 - stem
	if s >= 2 {
		out += stroke(f("M32 45V%g", top), leaf, 2.4)
	}
	blade := func(y, dir, size float64) string {
		return path(f("M32 %gq%g %g %g %gq%g %g %g %gZ", y, dir*size*.4, -size*.75, dir*size, -size*.55, -dir*size*.15, size*.65, -dir*size, size*.55), green)
	}
	if s >= 2 {
		out += blade(top+1, 1, 5+float64(min(s, 5)))
	}
	if s >= 3 {
		out += blade(top+3, -1, 4+float64(min(s, 5)))
	}
	if s >= 5 {
		out += blade(top+11, 1, 9)
	}
	if s >= 6 {
		out += blade(top+13, -1, 9)
	}
	if s == 6 {
		out += circle(32, top-1, 2.6, gold)
	}
	if s >= 7 {
		petals := ""
		for i := range 8 {
			a := float64(i) * math.Pi / 4
			petals += circle(32+5*math.Cos(a), top-3+5*math.Sin(a), 2.6, orange)
		}
		out += petals + circle(32, top-3, 3.8, gold)
	}
	out += path("M19 44h26l-3 15a2 2 0 0 1-2 2H24a2 2 0 0 1-2-2Z", clay) + rect(17, 41, 30, 5, 2, "#cf7a4b")
	if s == 1 {
		out += stroke("M24 42q5-3 10-1", "#5c3a26", 2)
	}
	if s == 8 {
		out += sparkle(10, 18, 4, gold) + sparkle(54, 12, 4, gold) + sparkle(54, 30, 2.6, gold)
	}
	return out
}

func egg(s int) string {
	nest := stroke("M9 48q23 15 46 0", brown, 5) + stroke("M12 54h9M43 54h9", "#6d4630", 2.6)
	shell := "#efe6d2"
	chick := func(y float64) string {
		return path(f("M21 %gl-2 -9 7 4ZM43 %gl2 -9 -7 4Z", y-8, y-8), green) + circle(32, y, 12.5, green) + circle(27.5, y-2, 2.2, ink) + circle(36.5, y-2, 2.2, ink) + path(f("M29 %gh6l-3 3.5Z", y+3), gold)
	}
	switch {
	case s <= 4:
		body := path("M32 12c9 0 15 13 15 23s-7 16-15 16-15-6-15-16 6-23 15-23Z", shell)
		if s == 2 {
			body += stroke("M31 24l3 3", "#a9a08d", 1.6)
		}
		if s == 3 {
			body += stroke("M28 26l4 4 4-3", "#8f8775", 1.8)
		}
		if s == 4 {
			body += stroke("M20 31l5 4 4-5 5 5 5-4 5 3", "#8f8775", 2)
		}
		return nest + body
	case s == 5:
		return chick(36) + path("M19 42l5 4 4-4 4 4 4-4 4 4 5-4v4c-2 7-8 9-13 9s-11-2-13-9Z", shell) + path("M22 26l10-8 10 8-5 2-5-2-5 2Z", shell) + nest
	case s == 6:
		return chick(34) + path("M19 44l6 3 7-3 7 3 6-3c0 7-6 10-13 10s-13-3-13-10Z", shell) + path("M23 19l9-7 9 7-4 2-5-2-5 2Z", shell) + nest
	case s == 7:
		return chick(33) + path("M21 47l11 3 11-3c-1 5-6 7-11 7s-10-2-11-7Z", shell) + nest
	}
	return stroke("M17 34l-8 5M47 34l8 5", green, 5) + chick(30) + path("M21 48l11 3 11-3c-1 5-6 7-11 7s-10-2-11-7Z", shell) + nest + sparkle(11, 15, 4, gold) + sparkle(52, 12, 3, gold)
}

func coffee(s int) string {
	out := shadow(16)
	level := float64(s-1) / 7
	top := 54 - level*30
	if s > 1 {
		out += path(f("M15 %gh32l-1.5 %gq-.3 3-3 3H19.5q-2.7 0-3-3Z", top, 57-top-3), "#7b4a2e")
		if s >= 7 {
			out += rect(15, top, 32, 3.5, 1, "#c79a72")
		}
	}
	out += stroke("M11 19h40l-3 37q-.3 4-4 4H18q-3.700 0-4-4Z", steel, 3.6) + stroke("M50 26h4a7 7 0 0 1 0 15h-5", steel, 3.6)
	if s == 8 {
		out += path("M31 28c-2-3-6-1-5 2 .6 2 3 4 5 5 2-1 4.400-3 5-5 1-3-3-5-5-2Z", "#f4eadb")
	}
	if s >= 5 {
		out += steam(31, s-4, 15)
	}
	return out
}

func fire(s int) string {
	out := ""
	if s >= 4 {
		r := 8 + float64(s-4)*2.6
		out += circle(32, 46-r*.3, r, "#5a3320")
	}
	if s == 2 {
		out += stroke("M32 42q-3-4 0-7q3-4 0-8", "#8d9690", 1.8)
	}
	if s == 3 {
		out += stroke("M32 44q-2-3 0-5", "#8d9690", 1.8)
	}
	if s >= 4 {
		k := float64(s-3) / 5
		flame := func(h, w float64, color string) string {
			return path(f("M32 %gc%g %g %g %g %g %gc0 %g %g %g %g %gc%g 0 %g %g %g %gc0 %g %g %g %g %gZ", 50-h, w*.3, h*.3, w, h*.5, w, h*.72, h*.2, -w*.45, h*.28, -w, h*.28, -w*.55, -w, -h*.08, -w, -h*.28, -h*.22, w*.7, -h*.42, w, -h*.72), color)
		}
		out += flame(10+26*k, 4+9*k, orange) + flame(6+15*k, 2.5+5*k, gold)
	}
	if s == 8 {
		out += circle(14, 12, 1.5, gold) + circle(50, 16, 1.5, gold) + circle(44, 6, 1.2, gold) + circle(21, 6, 1.2, orange)
	}
	out += circle(9, 54, 3.4, "#626a65") + circle(55, 54, 3.4, "#626a65") + circle(20, 57, 3, "#555c57") + circle(44, 57, 3, "#555c57")
	out += f(`<g stroke-linecap="round" stroke-width="6.5"><path d="M14 56L50 46" stroke="%s"/><path d="M14 46L50 56" stroke="%s"/></g>`, brown, "#a06a45")
	if s == 3 {
		out += stroke("M24 50h16", red, 2.4)
	}
	return out
}

func fishShape(x, y, size float64, color string) string {
	return path(f("M%g %gq%g %g %g 0q%g %g %g 0Z", x-size, y, size, -size*.9, size*2, -size, size*.9, -size*2)+f("M%g %gl%g %gv%gZ", x-size+1, y, -size*.7, -size*.6, size*1.2), color) + circle(x+size*.45, y-size*.12, size*.17, ink)
}

func aquarium(s int) string {
	out := shadow(21)
	if s >= 2 {
		y := 51 - float64(min(s, 5)-1)*8
		if s == 5 {
			y = 16
		} else if s > 5 {
			y = 15
		}
		out += path(f("M12 %gh40v%gq0 3-3 3H15q-3 0-3-3Z", y, 54-y), "#3e6a90")
	}
	if s >= 3 {
		out += path("M12 49q10-3 20-1t20-1v7q0 3-3 3H15q-3 0-3-3Z", "#d8b77b")
	}
	if s >= 4 {
		out += stroke("M20 50q-4-8 0-14q3-5 0-9", leaf, 2.4) + stroke("M45 50q4-8 0-13q-3-4 0-8", leaf, 2.4)
	}
	if s >= 5 {
		out += fishShape(31, 31, 7, orange)
	}
	if s >= 6 {
		out += ring(46, 19, 1.6, 1, "#cfe2ee") + ring(49, 13, 2, 1, "#cfe2ee")
	}
	if s >= 7 {
		out += fishShape(23, 43, 5, gold)
	}
	if s == 8 {
		out += fishShape(41, 42, 5, purple)
	}
	return out + stroke("M9 8h46M12 8v47q0 5 5 5h30q5 0 5-5V8", steel, 3.2)
}

func sky(s int) string {
	t := float64(s-1) / 7
	id := f("s%d", s)
	top, bottom := mix("#0e1533", "#4b95d9", t), mix("#1b2a52", "#8cc1e8", t)
	out := f(`<defs><linearGradient id="%s" x2="0" y2="1"><stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient><clipPath id="c%s"><rect x="5" y="5" width="54" height="54" rx="12"/></clipPath></defs><rect x="5" y="5" width="54" height="54" rx="12" fill="url(#%s)"/><g clip-path="url(#c%s)">`, id, top, bottom, id, id, id)
	if s <= 4 {
		stars := fade(1-t*1.1, circle(14, 16, 1, "#fff")+circle(26, 11, .9, "#fff")+circle(21, 26, .8, "#fff")+circle(36, 20, 1, "#fff")+circle(49, 29, .9, "#fff"))
		out += stars
	}
	if s <= 3 {
		out += circle(45, 17, 5, mix("#f6f2de", "#5a6a8c", float64(s-1)/2.4))
	}
	sunY := []float64{0, 0, 52, 43, 36, 31, 25, 21}[s-1]
	if s >= 3 {
		out += circle(32, sunY, 8, gold)
	}
	if s == 8 {
		for i, c := range []string{red, orange, gold, green, blue} {
			out += ring(32, 52, 26-float64(i)*2.6, 2.4, c)
		}
	}
	out += path("M0 50q16-9 32-3t32-3v21H0Z", mix("#17382f", "#3f9a6b", t))
	if s >= 6 {
		x := 15 + float64(s-6)*3
		out += circle(x, 21, 4, "#eef3f6") + circle(x+5, 19, 5, "#eef3f6") + circle(x+10, 21, 4, "#eef3f6")
	}
	return out + "</g>"
}

func bulb(s int) string {
	t := float64(s-1) / 7
	out := ""
	if s >= 2 {
		out += fade(.15+t*.45, circle(32, 25, 22, gold))
	}
	glass := mix("#3b413e", "#ffe08a", t)
	out += path("M32 5a19 19 0 0 0-11 34c2 2 3 4 3 7h16c0-3 1-5 3-7a19 19 0 0 0-11-34Z", glass)
	out += path("M32 5a19 19 0 0 0-11 34c2 2 3 4 3 7h16c0-3 1-5 3-7a19 19 0 0 0-11-34Z", "none")
	out += stroke("M32 5a19 19 0 0 0-11 34c2 2 3 4 3 7h16c0-3 1-5 3-7a19 19 0 0 0-11-34Z", mix("#606864", "#ffeeb5", t), 1.8)
	out += stroke("M27 45l-3-13 4 3 4-5 4 5 4-3-3 13", mix("#7f8884", "#b9892e", t), 1.8)
	out += rect(24, 46, 16, 4, 1.5, "#a9b3ad") + rect(25, 51, 14, 3.4, 1.5, "#8d9690") + rect(28, 55, 8, 3, 1.5, "#6f7772")
	if s >= 7 {
		rays := 5
		if s == 8 {
			rays = 9
		}
		for i := range rays {
			a := math.Pi + math.Pi*float64(i+1)/float64(rays+1)
			out += stroke(f("M%.1f %.1fL%.1f %.1f", 32+23*math.Cos(a), 25+23*math.Sin(a), 32+29*math.Cos(a), 25+29*math.Sin(a)), gold, 2.2)
		}
	}
	return out
}

func rocket(s int) string {
	lift := []float64{0, 0, 0, 0, -1, -3, -5, -7}[s-1]
	out := ""
	if s <= 6 {
		out += rect(14, 57, 36, 3, 1.5, "#5e6661") + rect(44, 34, 2.6, 24, 1, "#5e6661")
	}
	if s == 4 || s == 5 {
		out += circle(23, 56, 4, "#6f7772") + circle(32, 57, 4.4, "#7d8580") + circle(41, 56, 4, "#6f7772")
	}
	if s == 7 {
		out += `<ellipse cx="22" cy="59" rx="8" ry="2.6" fill="#7d8580"/><ellipse cx="42" cy="59" rx="8" ry="2.6" fill="#6f7772"/>`
	}
	if s >= 6 {
		out += circle(9, 19, 1, "#fff") + circle(55, 12, 1, "#fff") + circle(54, 40, 1, "#fff") + circle(11, 41, .9, "#fff")
	}
	if s == 8 {
		out += circle(53, 56, 8, blue) + circle(13, 53, 1, "#fff")
	}
	body := ""
	if s >= 3 {
		h := 3 + float64(s)*1.8
		body += path(f("M26 49h12l-6 %gZ", h+4), orange) + path(f("M28.500 49h7l-3.500 %gZ", h), gold)
	}
	window := []string{"#39424a", gold, blue, blue, blue, blue, blue, blue}[s-1]
	body += path("M24 34l-8 13h9ZM40 34l8 13h-9Z", red)
	body += path("M32 8c7 6 10 16 10 26 0 5-1 10-2 15H24c-1-5-2-10-2-15 0-10 3-20 10-26Z", "#dfe5e0")
	body += path("M32 8c3 3 6 7 7.500 11h-15C26 15 29 11 32 8Z", "#aeb8b2")
	body += circle(32, 28, 5.2, "#7b8782") + circle(32, 28, 3.6, window) + rect(24, 46, 16, 3.4, 1, "#7b8782")
	return out + f(`<g transform="translate(0 %g)">%s</g>`, lift, body)
}

func robotHead(y float64, s int) string {
	out := ""
	antenna := "#6f7772"
	if s >= 4 {
		antenna = gold
	}
	out += rect(31, y-8, 2, 8, 1, "#8d9690") + circle(32, y-9, 3, antenna)
	if s == 8 {
		out = fade(.3, circle(32, y-9, 7, gold)) + out
	}
	out += rect(7, y+7, 5, 11, 2.4, "#8d9690") + rect(52, y+7, 5, 11, 2.4, "#8d9690")
	out += rect(11, y, 42, 27, 9, "#d8e0db") + rect(16, y+5, 32, 17, 6, "#162029")
	return out
}

func robot(s int) string {
	y := 12.0
	out := shadow(15) + robotHead(y, s) + rect(23, 41, 18, 15, 5, "#aebbb4")
	eyeY, mouth := y+11, green
	switch {
	case s <= 3:
		eye := "#d9b24a"
		if s == 1 {
			eye = "#5f6b66"
		}
		out += stroke(f("M21 %gh7M36 %gh7", eyeY, eyeY), eye, 2.4)
	case s == 8:
		out += stroke(f("M20 %gl4-4 4 4M36 %gl4-4 4 4", eyeY+1, eyeY+1), green, 2.4)
	default:
		out += circle(24.5, eyeY, 2.4, green) + circle(39.500, eyeY, 2.4, green)
	}
	switch {
	case s == 1:
		out += stroke(f("M27 %gq5-3 10 0", y+18), "#5f6b66", 2)
	case s <= 3:
		out += stroke(f("M27 %gh10", y+17), "#d9b24a", 2)
	case s == 4:
		out += stroke(f("M27 %gh10", y+17), mouth, 2)
	case s <= 6:
		out += stroke(f("M26 %gq6 %g 12 0", y+16, float64(s-3)), mouth, 2)
	default:
		out += stroke(f("M25 %gq7 7 14 0", y+15), mouth, 2.4)
	}
	if s >= 7 {
		out += stroke("M9 30l-5-8M55 30l5-8", "#8d9690", 3)
	}
	if s <= 2 {
		out += f(`<text x="53" y="11" font-family="sans-serif" font-size="8" font-weight="700" fill="#8d9690">z</text>`)
		out += f(`<text x="58" y="6" font-family="sans-serif" font-size="6" font-weight="700" fill="#8d9690">z</text>`)
	}
	return out
}

func robotBuild(s int) string {
	out := shadow(16)
	wrench := stroke("M49 15l6-6", "#8d9690", 2.6) + stroke("M53 7l4 4", "#8d9690", 2.6)
	if s == 1 {
		return out + rect(11, 51, 11, 6, 2, "#8d9690") + rect(25, 52, 12, 5, 2, "#aebbb4") + circle(44, 54, 3.4, "#7d8580") + stroke("M34 44l4 4M38 44l-4 4", "#8d9690", 2)
	}
	if s == 2 {
		return out + wrench + rect(13, 50, 11, 6, 2, "#8d9690") + rect(20, 54, 9, 4, 1.5, "#aebbb4") + rect(34, 53, 9, 4, 1.5, "#aebbb4") + circle(48, 52, 3, "#7d8580")
	}
	if s == 3 {
		return out + wrench + rect(22, 47, 20, 10, 4, "#aebbb4") + rect(24, 56, 6, 3, 1, "#8d9690") + rect(34, 56, 6, 3, 1, "#8d9690")
	}
	body := rect(23, 42, 18, 15, 5, "#aebbb4")
	if s == 4 {
		return out + wrench + body + rect(14, 18, 36, 27, 9, "#d8e0db")
	}
	out += body + rect(12, 16, 40, 27, 9, "#d8e0db")
	if s < 8 {
		out += wrench
	}
	out += rect(17, 21, 30, 17, 6, "#162029")
	if s == 6 {
		out += stroke("M23 30h6M35 30h6", "#5f6b66", 2)
	}
	if s == 7 || s == 8 {
		out += circle(25, 28, 2.4, green) + circle(39, 28, 2.4, green) + stroke("M27 33q5 3 10 0", green, 2)
	}
	if s == 8 {
		out += rect(30, 8, 2, 8, 1, "#8d9690") + fade(.3, circle(31, 7, 6, gold)) + circle(31, 7, 3, gold) + rect(26, 47, 12, 4, 1.5, green)
	}
	return out
}

func battery(s int) string {
	out := ""
	if s == 8 {
		out += fade(.25, rect(2, 12, 60, 40, 12, green))
	}
	color := []string{red, red, red, gold, gold, green, green, green}[s-1]
	width := []float64{3, 7, 16, 21, 26, 32, 37, 44}[s-1]
	out += frame(6, 17, 50, 30, 8, 3.4, "#68716c", "none") + rect(58, 26, 3.4, 12, 1.5, "#68716c") + rect(9.5, 20.5, width, 23, 4, color)
	if s >= 2 && s < 8 {
		bolt := "#fff3c2"
		if s >= 6 {
			bolt = ink
		} else if s <= 3 {
			bolt = gold
		}
		out += path("M34 21L25 34h6l-3 9 10-14h-6Z", bolt)
	}
	if s == 8 {
		out += stroke("M21 32l7 7 14-14", ink, 4.4)
	}
	return out
}

func terminal(s int) string {
	border := "#333a36"
	if s == 8 {
		border = green
	}
	out := frame(5, 8, 54, 48, 7, 1.6, border, "#0a0d0c") + rect(11, 14, 18, 3, 1, "#68716c")
	lines := min(s-1, 3)
	for i := range lines {
		y := 21 + float64(i)*6
		out += stroke(f("M12 %gl1.500 1.500 3-3", y+1.5), green, 1.3) + rect(19, y, 22, 2.6, 1, "#68716c")
	}
	if s >= 2 {
		out += rect(11, 40, 42, 5, 1, "#2a2f2c")
		color := gold
		if s == 8 {
			color = green
		}
		out += rect(11, 40, 42*float64(s)/8, 5, 1, color)
	}
	if s == 8 {
		out += rect(11, 48, 24, 3, 1, green)
	}
	return out
}

func laptop(s int) string {
	screen := "#090c0b"
	if s >= 5 {
		screen = "#163b2e"
	}
	out := shadow(26) + frame(11, 10, 42, 32, 4, 2.8, "#68716c", screen)
	switch {
	case s == 2:
		out += rect(16, 15, 2, 4, .5, "#d8e0db")
	case s == 3 || s == 4:
		a := float64(s-2) * 95
		out += f(`<circle cx="32" cy="24" r="6" fill="none" stroke="%s" stroke-width="2.4" stroke-dasharray="%g 40" stroke-linecap="round" transform="rotate(-90 32 24)"/>`, green, a/360*37.7) + rect(25, 34, 14, 2, 1, "#3c6b58")
	case s == 5:
		out += circle(32, 22, 4, green) + rect(23, 29, 18, 3, 1.5, green)
	case s == 6:
		out += rect(16, 15, 12, 9, 1, "#3c6b58") + rect(30, 15, 18, 3.5, 1, "#3c6b58") + rect(30, 21, 14, 3.5, 1, "#3c6b58") + rect(16, 28, 32, 8, 1, "#2a5a48")
	default:
		out += rect(16, 16, 22, 2.4, 1, green) + rect(16, 21, 16, 2.4, 1, green) + rect(16, 26, 26, 2.4, 1, green) + rect(16, 31, 12, 2.4, 1, green)
		if s == 8 {
			out += circle(44, 32, 6, green) + stroke("M41 32l2.500 2.500 4.500-5", ink, 2)
		}
	}
	return out + path("M5 44h54l-3 7q-.5 1-2 1H10q-1.500 0-2-1Z", "#7d8580")
}

func wifi(s int) string {
	level := s - 1
	colors := func(i int) string {
		if level >= i {
			if s == 8 {
				return green
			}
			return mix("#3f7f62", green, float64(level)/7)
		}
		return "#4a514d"
	}
	dot := "#68716c"
	if s >= 2 {
		dot = green
	}
	out := ""
	if s == 8 {
		out += stroke("M9 54a29 29 0 1 1 46 0", "#2f6f55", 2)
	}
	out += stroke("M7 25a36 36 0 0 1 50 0", colors(6), 5.4)
	out += stroke("M14 33a26 26 0 0 1 36 0", colors(4), 5.4)
	out += stroke("M22 41a15 15 0 0 1 20 0", colors(2), 5.4)
	out += circle(32, 49, 4.4, dot)
	if s == 1 {
		out += stroke("M43 45l9 9M52 45l-9 9", red, 3.4)
	}
	return out
}

func git(s int) string {
	y, up := 44.0, 28.0
	out := rect(6, y-1.2, 52, 2.4, 1.2, "#3c4340")
	node := func(x, cy float64, color string) string {
		return f(`<circle cx="%g" cy="%g" r="4.2" fill="#14211b" stroke="%s" stroke-width="2.8"/>`, x, cy, color)
	}
	if s == 1 {
		return out + f(`<circle cx="10" cy="%g" r="4.4" fill="none" stroke="#7d8580" stroke-width="2.4" stroke-dasharray="2 2"/>`, y)
	}
	out += node(10, y, green)
	if s >= 3 {
		out += node(21, y, green)
	}
	if s >= 4 {
		out += stroke(f("M21 %gq3-14 10-16", y), purple, 2.4) + node(32, up, purple)
	}
	if s >= 5 {
		out += stroke(f("M32 %gh10", up), purple, 2.4) + node(42, up, purple)
	}
	if s >= 6 {
		out += stroke(f("M42 %gh9", up), purple, 2.4)
	}
	if s >= 7 {
		out += stroke(f("M21 %gh33", y), green, 2.4) + stroke(f("M51 %gq3 2 3 14", up), purple, 2.4) + node(54, y, green)
	}
	if s == 6 {
		out += node(51, up, purple)
	}
	if s >= 7 {
		out += node(51, up, purple)
	}
	if s == 8 {
		out += rect(38, 7, 22, 12, 3, green) + rect(42, 11, 14, 2.4, 1, ink) + stroke("M54 19v21", green, 1.4)
	}
	return out
}

func board(s int) string {
	lit := func(i int) string {
		if s > i {
			return green
		}
		return "#2f4a3d"
	}
	out := rect(4, 4, 56, 56, 6, "#10231b")
	paths := []string{"M21 24H9V14", "M21 32H6", "M21 40H9V50", "M43 24h12V14", "M43 32h15", "M43 40h12V50", "M32 21V6", "M32 43v15"}
	ends := [][2]float64{{9, 14}, {6, 32}, {9, 50}, {55, 14}, {58, 32}, {55, 50}, {32, 6}, {32, 58}}
	for i, p := range paths {
		c := lit(i + 1)
		out += stroke(p, c, 2.2) + circle(ends[i][0], ends[i][1], 2.2, c)
	}
	if s >= 6 {
		out = fade(1, out)
	}
	chip := "#2a2f2c"
	core := mix("#39413d", green, float64(s-1)/7)
	out += rect(20, 20, 24, 24, 3, chip) + rect(26, 26, 12, 12, 2, core)
	for i := range 4 {
		y := 24 + float64(i)*5.3
		out += rect(18, y, 2, 2, .5, "#a9b3ad") + rect(44, y, 2, 2, .5, "#a9b3ad")
	}
	return out
}

func main() {
	figures := map[string]func(int) string{
		"pflanze": plant, "ei": egg, "kaffee": coffee, "lagerfeuer": fire, "aquarium": aquarium, "himmel": sky, "gluehbirne": bulb, "rakete": rocket,
		"roboter": robot, "roboter-bau": robotBuild, "akku": battery, "terminal": terminal, "laptop": laptop, "wlan": wifi, "git": git, "platine": board,
	}
	dir := filepath.Join("web", "static", "figures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	for key, draw := range figures {
		for s := 1; s <= 8; s++ {
			body := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">` + draw(s) + "</svg>\n"
			if strings.Contains(body, "%!") {
				log.Fatalf("%s %d: bad format", key, s)
			}
			if err := os.WriteFile(filepath.Join(dir, f("%s-%d.svg", key, s)), []byte(body), 0o644); err != nil {
				log.Fatal(err)
			}
		}
	}
}
