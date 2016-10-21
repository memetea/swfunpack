package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

var dir = flag.String("d", "", "input directory")
var file = flag.String("f", "", "file name")
var output = flag.String("o", "./output", "output directory or file")
var exclude = flag.String("e", "", "pass file if exclude in decompressed content")

type swf struct {
	Version byte
	Content io.Reader
}

//return content reader of decompressed swf
func unpack(r io.Reader) (io.Reader, error) {
	//check first 3 bytes
	var chunk = make([]byte, 3, 3)
	if _, err := r.Read(chunk); err != nil {
		return nil, err
	}
	if !bytes.Equal(chunk, []byte("FWS")) && !bytes.Equal(chunk, []byte("CWS")) {
		return nil, fmt.Errorf("invalid file header")
	}

	if bytes.Equal(chunk, []byte("CWS")) {
		//compressed file
		//skip version && file bytes
		chunk = make([]byte, 5, 5)
		if _, err := r.Read(chunk); err != nil {
			return nil, err
		}
		content, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return zlib.NewReader(bytes.NewReader(content))
	}

	//FWS: uncompressed
	br := bufio.NewReader(r)

	//skip version
	_, err := br.ReadByte()
	if err != nil {
		return nil, err
	}
	//read file length

	var fileLen uint32
	if err := binary.Read(br, binary.LittleEndian, &fileLen); err != nil {
		return nil, err
	}
	content, err := ioutil.ReadAll(br)
	if err != nil {
		return nil, err
	}
	if i := bytes.Index(content, []byte("CWS")); i >= 0 {
		return unpack(bytes.NewReader(content[i:]))
	}

	if i := bytes.Index(content, []byte("FWS")); i >= 0 {
		return unpack(bytes.NewReader(content[i:]))
	}

	return bytes.NewReader(content), nil
}

func processFile(p string) (*swf, error) {
	r, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var instance swf
	instance.Content, err = unpack(r)
	if err != nil {
		return nil, err
	}
	r.Seek(3, os.SEEK_SET)
	instance.Version, err = bufio.NewReader(r).ReadByte()
	if err != nil {
		return nil, err
	}
	return &instance, nil
}

func parseMixStr(str []byte) []byte {
	var buf bytes.Buffer
	for i := 0; i < len(str); {
		if str[i] == '\\' {
			if i < len(str)-2 && str[i+1] == 'x' {
				var v byte
				fmt.Sscanf(string(str[i+2:i+4]), "%x", &v)
				buf.WriteByte(v)
				i += 4
			} else if i < len(str)-2 && str[i+1] == '\\' {
				buf.Write(str[i : i+2])
				i += 2
			} else {
				buf.WriteByte(str[i])
				i++
			}
		} else {
			buf.WriteByte(str[i])
			i++
		}
	}
	return buf.Bytes()
}

func main() {
	flag.Parse()
	if len(*dir) == 0 && len(*file) == 0 {
		fmt.Println("invalid arguments")
		return
	}

	ex := []byte(*exclude)
	if len(ex) > 0 {
		ex = parseMixStr(ex)
		fmt.Printf("ex:%s\n ex:% x\n", ex, ex)
	}

	outputs := make(map[string]swf)
	if len(*dir) > 0 {
		fmt.Printf("process dir:%s\n", *dir)
		filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
			if !info.IsDir() && filepath.Ext(path) == ".swf" {
				//fmt.Printf("process %s\n", path)
				instance, err := processFile(path)
				if err != nil {
					fmt.Printf("waning: process %s err:%v\n", path, err)
				} else {
					outputs[info.Name()] = *instance
				}
			}
			return nil
		})
	}

	if len(*file) > 0 /*&& filepath.Ext(*file) == ".swf"*/ {
		fmt.Printf("process %s...\n", *file)
		instance, err := processFile(*file)
		if err != nil {
			fmt.Printf("processError: %s\n", err)
		}

		outputs[filepath.Base(*file)] = *instance
	}

	wd, _ := os.Getwd()
	//fmt.Printf("%v\n\n", outputs)
	if len(*output) > 0 {
		os.MkdirAll(*output, 0777)
		for k, v := range outputs {
			p := filepath.Join(wd, *output, k)
			content, err := ioutil.ReadAll(v.Content)
			if err != nil {
				fmt.Printf("read unpacked content failed:%v\n", err)
				continue
			}
			if len(ex) > 0 && bytes.Index(content, ex) >= 0 {
				fmt.Printf("exclude file:%s\n", p)
				continue
			}

			fw, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0444)
			if err != nil {
				fmt.Printf("open file %s error:\n\t%s\n", p, err)
				continue
			}
			defer fw.Close()
			fw.Write([]byte("FWS"))
			fw.Write([]byte{v.Version})
			fileLen := int32(len(content)) + 8
			binary.Write(fw, binary.LittleEndian, &fileLen)
			_, err = fw.Write(content)
			if err != nil {
				fmt.Printf("write content error:%v\n", err)
			}
			//fmt.Printf("write file %s len = %d\n", p, fileLen)
			fw.Close()
		}
	}
}
