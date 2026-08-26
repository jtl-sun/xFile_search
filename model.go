package main

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"time"
)

const (
	appName        = "xFile_search"
	appVersion     = "0.1.25"
	indexMagic     = "XFSIDX03"
	indexVersion   = uint32(3)
	maxPathBytes   = 1 << 20
	defaultDisplay = 1500
)

type Entry struct { Path string; FoldName string; NameStart int; IsDir bool }
func NewEntry(path string,isDir bool) Entry { clean:=filepath.Clean(path); nameStart:=strings.LastIndexAny(clean,`\/`); if nameStart<0{nameStart=0}else{nameStart++}; name:=clean;if nameStart>=0&&nameStart<len(clean){name=clean[nameStart:]};return Entry{Path:clean,FoldName:strings.ToLower(name),NameStart:nameStart,IsDir:isDir} }
func (e Entry) Name() string { if e.NameStart>=0&&e.NameStart<len(e.Path){return e.Path[e.NameStart:]};return e.Path }

type MappedIndex struct { data []byte; closeMap func() error; count uint64; offsetsAt uint64; builtAt time.Time; roots []string; sourcePath string }
type IndexSnapshot struct { Entries []Entry; Mapped *MappedIndex; BuiltAt time.Time; Roots []string; Source string }
func (s *IndexSnapshot) Len() int { if s==nil{return 0};if s.Mapped!=nil{if s.Mapped.count>uint64(^uint(0)>>1){return int(^uint(0)>>1)};return int(s.Mapped.count)};return len(s.Entries) }
func (s *IndexSnapshot) Close() error { if s==nil||s.Mapped==nil||s.Mapped.closeMap==nil{return nil};err:=s.Mapped.closeMap();s.Mapped.closeMap=nil;s.Mapped.data=nil;return err }
func (s *IndexSnapshot) EntryAt(id uint32)(Entry,bool){if s==nil{return Entry{},false};if s.Mapped==nil{if int(id)>=len(s.Entries){return Entry{},false};return s.Entries[id],true};p,nameStart,isDir,ok:=s.Mapped.record(id);if !ok{return Entry{},false};path:=string(p);if nameStart<0||nameStart>len(path){nameStart=0};return Entry{Path:path,NameStart:nameStart,IsDir:isDir},true}
func (m *MappedIndex) record(id uint32)(path []byte,nameStart int,isDir bool,ok bool){if m==nil||uint64(id)>=m.count{return nil,0,false,false};tablePos:=m.offsetsAt+uint64(id)*8;if tablePos+8>uint64(len(m.data)){return nil,0,false,false};rec:=binary.LittleEndian.Uint64(m.data[tablePos:tablePos+8]);if rec+9>uint64(len(m.data))||rec>=m.offsetsAt{return nil,0,false,false};flags:=m.data[rec];ns:=binary.LittleEndian.Uint32(m.data[rec+1:rec+5]);n:=binary.LittleEndian.Uint32(m.data[rec+5:rec+9]);start:=rec+9;end:=start+uint64(n);if end>uint64(len(m.data))||end>m.offsetsAt||uint64(ns)>uint64(n){return nil,0,false,false};return m.data[start:end],int(ns),flags&1!=0,true}

type FilterMode int
const ( FilterAll FilterMode=iota; FilterFiles; FilterFolders )
type SearchStep struct { Query string; IDs []uint32 }
type SearchResult struct { IDs []uint32; Query string; Elapsed time.Duration; Scanned int; Canceled bool }
