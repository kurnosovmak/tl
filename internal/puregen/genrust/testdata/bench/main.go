// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/VKCOM/tl/target/rust-bench/go_codec/basictl"
	"github.com/VKCOM/tl/target/rust-bench/go_codec/tlbench"
)

type message interface {
	WriteTL1General([]byte) ([]byte, error)
	ReadTL1([]byte) ([]byte, error)
	WriteTL2([]byte, *basictl.TL2WriteContext) []byte
	ReadTL2([]byte, *basictl.TL2ReadContext) ([]byte, error)
	WriteJSONGeneral(*basictl.JSONWriteContext, []byte) ([]byte, error)
	ReadJSONGeneral(*basictl.JSONReadContext, *basictl.JsonLexer) error
}

type codec struct {
	name  string
	write func([]byte) []byte
	read  func([]byte)
}

func measure(f func()) float64 {
	batch := func(repeats int) time.Duration {
		start := time.Now()
		for i := 0; i < repeats; i++ {
			f()
		}
		return time.Since(start)
	}
	count := 1
	for {
		elapsed := batch(count)
		if elapsed >= 12*time.Millisecond {
			count = max(1, int(float64(count)*float64(60*time.Millisecond)/float64(elapsed)))
			break
		}
		count *= 2
	}
	var samples [7]float64
	for i := range samples {
		samples[i] = float64(batch(count).Nanoseconds()) / float64(count)
	}
	slices.Sort(samples[:])
	return samples[len(samples)/2]
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func consumed(rest []byte, err error) {
	check(err)
	if len(rest) != 0 {
		panic("unconsumed bytes")
	}
}

func run(source, dest message, dir, mode string) {
	var tl2write basictl.TL2WriteContext
	var tl2read basictl.TL2ReadContext
	var jsonWrite basictl.JSONWriteContext
	var jsonRead basictl.JSONReadContext
	for _, codec := range []codec{
		{"TL1", func(buf []byte) []byte { out, err := source.WriteTL1General(buf); check(err); return out },
			func(in []byte) { consumed(dest.ReadTL1(in)) }},
		{"TL2", func(buf []byte) []byte { return source.WriteTL2(buf, &tl2write) },
			func(in []byte) { consumed(dest.ReadTL2(in, &tl2read)) }},
		{"JSON", func(buf []byte) []byte { out, err := source.WriteJSONGeneral(&jsonWrite, buf); check(err); return out },
			func(in []byte) {
				lexer := basictl.JsonLexer{Data: in}
				check(dest.ReadJSONGeneral(&jsonRead, &lexer))
				lexer.Consumed()
				check(lexer.Error())
			}},
	} {
		path := func(lang string) string { return filepath.Join(dir, lang+"."+codec.name) }
		if mode == "prepare" {
			check(os.WriteFile(path("go"), codec.write(nil), 0644))
			continue
		}
		wire, err := os.ReadFile(path("rust"))
		check(err)
		codec.read(wire)
		expected, err := source.WriteTL1General(nil)
		check(err)
		actual, err := dest.WriteTL1General(nil)
		check(err)
		if !bytes.Equal(actual, expected) {
			panic(fmt.Sprintf("%s/%s: Rust input changed TL1 value", dir, codec.name))
		}
		if mode == "verify" {
			continue
		}
		wire, err = os.ReadFile(path("go"))
		check(err)
		buf := make([]byte, 0, len(wire))
		write := measure(func() { buf = codec.write(buf[:0]) })
		read := measure(func() { codec.read(wire) })
		runtime.KeepAlive(buf)
		runtime.KeepAlive(dest)
		fmt.Printf("%s,%d,%.3f,%.3f\n", codec.name, len(buf), write, read)
	}
}

func main() {
	runtime.GOMAXPROCS(1)
	newValue := map[string]func() message{
		"bytes": func() message { return &tlbench.BytesBytes{} },
		"ints":  func() message { return &tlbench.Ints{} },
		"row":   func() message { return &tlbench.RowBytes{} },
		"batch": func() message { return &tlbench.BatchBytes{} },
	}[os.Args[1]]
	dir, mode := os.Args[2], os.Args[3]
	source := newValue()
	input, err := os.ReadFile(filepath.Join(dir, "input.json"))
	check(err)
	lexer := basictl.JsonLexer{Data: input}
	check(source.ReadJSONGeneral(nil, &lexer))
	lexer.Consumed()
	check(lexer.Error())
	run(source, newValue(), dir, mode)
}
