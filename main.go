package main

import (
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/tarm/serial"
	"log"
	"os"
	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/conn/v3/gpio/gpioutil"
	"periph.io/x/host/v3"
	"time"
)

var options struct {
	GpioEnabled    bool   `long:"gpio-enabled" description:"Enable GPIO pulses based on ToD"`
	SecA           string `long:"gpio-seconds-a-pin" description:"Seconds alternating pin" default:"5"`
	SecPulse       string `long:"gpio-seconds-pulse-pin" description:"Seconds pulsing pin" default:"6"`
	SecTrigger     string `long:"gpio-seconds-trigger" description:"Seconds trigger pin" default:"17"`
	MinA           string `long:"gpio-minutes-a-pin" description:"Minutes alternating pin" default:"13"`
	MinPulse       string `long:"gpio-minutes-pulse-pin" description:"Minutes pulsing pin" default:"19"`
	MinTrigger     string `long:"gpio-minutes-trigger" description:"Minutes triggering pin" default:"27"`
	HourA          string `long:"gpio-hours-a-pin" description:"Hours alternating pin" default:"26"`
	HourPulse      string `long:"gpio-hours-pulse-pin" description:"Hours pulsing pin" default:"20"`
	HourTrigger    string `long:"gpio-hours-trigger" description:"Hours triggering pin" default:"22"`
	PulseDuration  int    `long:"gpio-pulse-duration" description:"Pulse duration, in milliseconds" default:"300"`
	InvertPolarity bool   `long:"gpio-invert-polarity" description:"Invert pulse polarity"`
	TimeZone       string `long:"gpio-timezone" default:"Europe/Helsinki"`
	Serial         string `long:"serial" default:"/dev/ttyUSB0"`
}
var parser = flags.NewParser(&options, flags.Default)

const pollSleep = 300 * time.Millisecond
const denoise = 50 * time.Millisecond
const debounce = 30 * time.Millisecond

const baseLen = time.Millisecond * 2
const startLen = 8 + 2
const zeroLen = 2 + 2
const oneLen = 5 + 2
const incrementDuration = (time.Millisecond * 2) * (zeroLen + (3 * oneLen))
const messageSpacing = 100 * time.Millisecond

func main() {
	var err error

	if _, err = parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			panic(err)
		}
	}

	pulser.Options()
	go pulser.Run()

	increment := []byte("SE")
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("Opening serial: %v", options.Serial)
	c := &serial.Config{Name: options.Serial, Baud: 19200}
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal(err)
	}

	locations := make([]*time.Location, 8)

	locations[1], _ = time.LoadLocation("Europe/Helsinki")
	locations[2], _ = time.LoadLocation("Europe/Moscow")
	locations[3], _ = time.LoadLocation("Asia/Shanghai")
	locations[4], _ = time.LoadLocation("Asia/Tokyo")
	locations[5], _ = time.LoadLocation("America/Los_Angeles")
	locations[6], _ = time.LoadLocation("America/New_York")
	locations[7], _ = time.LoadLocation("Etc/GMT")
	locations[0], _ = time.LoadLocation("Europe/Paris")

	time.Sleep(200 * time.Millisecond)
	sendClocks(locations, s)
	sendClocks(locations, s)

	t := time.NewTimer(minuteDuration())
	for {
		select {
		case <-t.C:
			log.Printf("Increment loop: %v", time.Now())
			s.Write(increment)
			time.Sleep(messageSpacing)
			sendClocks(locations, s)
			t.Reset(minuteDuration())
		}
	}
}

func serialListen(s *serial.Port) {
	buf := make([]byte, 64)
	log.Printf("Listening..")
	for {
		n, err := s.Read(buf)
		if err != nil {
			log.Fatalf("Error reading: %v", err)
		}
		log.Printf("%s", buf[:n])
	}
}

func sendClocks(locations []*time.Location, s *serial.Port) {
	t := time.Now().Add(time.Second)

	for i, l := range locations {
		local := t.In(l)
		hours := local.Hour()
		minutes := local.Minute()
		msg := fmt.Sprintf("S%d%d%d%d%d", i, minutes%10, minutes/10, hours%10, hours/10)
		log.Printf("  -> Sending: %v", msg)
		s.Write([]byte(msg))
		time.Sleep(messageSpacing)
	}
}

func minuteDuration() time.Duration {
	return time.Until(time.Now().Truncate(time.Minute).Add(time.Minute - incrementDuration))
}

type gpioPulser struct {
	enabled  bool
	secPins  *gpioPins
	minPins  *gpioPins
	hourPins *gpioPins
	duration time.Duration
	polarity gpio.Level
	loc      *time.Location
}

type gpioPins struct {
	a         gpio.PinIO
	pulse     gpio.PinIO
	trigger   gpio.PinIO
	direction gpio.Level
	c         chan bool
}

var pulser *gpioPulser

func init() {
	// Make sure periph is initialized.
	if _, err := host.Init(); err != nil {
		log.Printf("Error initializing GPIO: %v", err)
		return
	}

	p := &gpioPulser{
		duration: 500 * time.Millisecond,
		polarity: true,
	}

	pulser = p
}

func (p *gpioPulser) Options() {
	p.enabled = options.GpioEnabled
	if !p.enabled {
		return
	}
	p.duration = time.Duration(options.PulseDuration) * time.Millisecond
	p.polarity = gpio.Level(options.InvertPolarity)

	p.secPins = &gpioPins{
		a:       gpioreg.ByName(options.SecA),
		pulse:   gpioreg.ByName(options.SecPulse),
		trigger: gpioreg.ByName(options.SecTrigger),
	}

	p.minPins = &gpioPins{
		a:       gpioreg.ByName(options.MinA),
		pulse:   gpioreg.ByName(options.MinPulse),
		trigger: gpioreg.ByName(options.MinTrigger),
	}

	p.hourPins = &gpioPins{
		a:       gpioreg.ByName(options.HourA),
		pulse:   gpioreg.ByName(options.HourPulse),
		trigger: gpioreg.ByName(options.HourTrigger),
	}

	if err := p.secPins.init(p.polarity); err != nil {
		log.Printf("Error initializing second pulse pins: %v", err)
	}

	if err := p.minPins.init(p.polarity); err != nil {
		log.Printf("Error initializing minute pulse pins: %v", err)
	}

	if err := p.hourPins.init(p.polarity); err != nil {
		log.Printf("Error initializing hour pulse pins: %v", err)
	}

	var err error
	p.loc, err = time.LoadLocation(options.TimeZone)
	if err != nil {
		log.Printf("GPIO Pulser: Error loading time location: %v", err)
		p.enabled = false
	}
}

func (g *gpioPins) sendPulse(polarity gpio.Level, duration time.Duration) {
	g.pulse.Out(polarity)
	time.Sleep(duration)
	g.pulse.Out(!polarity)
	time.Sleep(duration)
	g.direction = !g.direction
	g.a.Out(g.direction)
}

func (g *gpioPins) listen(polarity gpio.Level, duration time.Duration) {
	for range g.c {
		g.sendPulse(polarity, duration)
	}
}

func (g *gpioPins) init(polarity gpio.Level) error {
	if g.a == nil {
		return fmt.Errorf("Alternating pin missing")
	}

	if g.pulse == nil {
		return fmt.Errorf("Pulsing pin missing")
	}

	if g.trigger == nil {
		return fmt.Errorf("Triggering pin missing")
	}

	g.a.Out(g.direction)
	g.pulse.Out(!polarity)
	g.c = make(chan bool)
	return nil
}

func (p *gpioPulser) secTrigger() {
	pin, _ := gpioutil.Debounce(p.secPins.trigger, denoise, debounce, gpio.RisingEdge)
	for {
		pin.WaitForEdge(-1)
		time.Sleep(debounce)
		if pin.Read() {
			log.Printf("Second trigger pin active")
			p.secPins.c <- true
		}
		time.Sleep(pollSleep)
	}
}

func (p *gpioPulser) minTrigger() {
	pin, _ := gpioutil.Debounce(p.minPins.trigger, denoise, debounce, gpio.RisingEdge)
	for {
		pin.WaitForEdge(-1)
		time.Sleep(debounce)
		if pin.Read() {
			log.Printf("Minute trigger pin active")
			p.minPins.c <- true
		}
		time.Sleep(pollSleep)
	}
}

func (p *gpioPulser) hourTrigger() {
	pin, _ := gpioutil.Debounce(p.hourPins.trigger, denoise, debounce, gpio.RisingEdge)
	for {
		pin.WaitForEdge(-1)
		time.Sleep(debounce)
		if pin.Read() {
			log.Printf("Hour trigger pin active")
			for i := 0; i < 60; i++ {
				p.minPins.c <- true
			}
			p.hourPins.c <- true
		}
		time.Sleep(pollSleep)
	}
}

// Run is executed as goroutine for output module
func (p *gpioPulser) Run() {
	if !p.enabled {
		return
	}

	secTimer := time.NewTimer(pulserSecondDuration())
	minTimer := time.NewTimer(pulserMinuteDuration())
	hourTimer := time.NewTimer(pulserHourDuration())

	go p.secPins.listen(p.polarity, p.duration)
	go p.minPins.listen(p.polarity, p.duration)
	go p.hourPins.listen(p.polarity, p.duration)
	go p.secTrigger()
	go p.minTrigger()
	go p.hourTrigger()

	for {
		select {
		case <-secTimer.C:
			p.secPins.c <- true
			secTimer.Reset(pulserSecondDuration())
			log.Printf("Second pulse sent")
		case <-minTimer.C:
			p.minPins.c <- true
			minTimer.Reset(pulserMinuteDuration())
			log.Printf("Minute pulse sent")
		case <-hourTimer.C:
			p.hourPins.c <- true
			hourTimer.Reset(pulserHourDuration())
			log.Printf("Hour pulse sent")
		}
	}
}

func pulserSecondDuration() time.Duration {
	return time.Until(time.Now().In(pulser.loc).Truncate(time.Second).Add(time.Second))
}

func pulserMinuteDuration() time.Duration {
	return time.Until(time.Now().In(pulser.loc).Truncate(time.Minute).Add(time.Minute))
}

func pulserHourDuration() time.Duration {
	return time.Until(time.Now().In(pulser.loc).Truncate(time.Hour).Add(time.Hour))
}
