# Stage 3: Streaming Optimization

**Goal**: Smooth playback with intelligent piece prioritization.

**Prerequisite**: Stage 2 complete (basic streaming works)

**End state**: Fast playback start, responsive seeking, efficient bandwidth usage

---

## The Problem

Stage 2 streaming has issues:
- **Slow start**: Video player needs header data (moov atom, EBML) before playback
- **Seek lag**: Jumping to new position waits for pieces at that offset
- **No read-ahead**: Sequential viewing downloads pieces just-in-time

anacrolix/torrent downloads pieces somewhat randomly based on availability. We need to prioritize pieces for streaming.

---

## Step 3.1: Understand Video Container Formats

**Context before implementation**:

**MP4/M4V**:
- Contains "moov" atom with metadata (duration, codec info, seek table)
- moov can be at START (fast start) or END (requires seeking to end first)
- Without moov, player can't even display duration

**MKV/WebM**:
- EBML header at start
- SeekHead element contains index
- Cues element has seek points
- Usually first 10-20MB contains all metadata

**Key insight**: Players need metadata before playback. If these pieces aren't downloaded, playback won't start even if middle content is available.

---

## Step 3.2: Design Priority Levels

anacrolix/torrent supports piece priorities:
- `PiecePriorityNone` - don't download
- `PiecePriorityNormal` - standard
- `PiecePriorityHigh` - prefer over normal
- `PiecePriorityReadahead` - aggressive download
- `PiecePriorityNow` - immediate need

**Strategy**:
```
[Header: 10MB]──────[Body]───────────────────[Footer: 5MB]
     HIGH           NORMAL                      HIGH

On playback/seek:
[───────][Urgent 2MB][Readahead 16MB][────────────────]
              NOW       READAHEAD       (deprioritize)
```

---

## Step 3.3: Prioritizer Component

Manages piece priorities for a file.

**Responsibilities**:
- Calculate which pieces cover header/footer regions
- Update priorities on seek
- Track current playback position

**Tasks**:
1. Create Prioritizer struct (needs torrent, file, piece length)
2. InitialPrioritize() - set header/footer to HIGH on file open
3. UpdateForSeek(offset) - reprioritize around new position
4. Helper: byteToPiece(offset) - convert byte offset to piece index
5. Helper: filePieceRange() - pieces that contain this file

**Configuration**:
```yaml
streaming:
  header_priority_bytes: 10485760   # 10MB
  footer_priority_bytes: 5242880    # 5MB
  readahead_bytes: 16777216         # 16MB
  urgent_buffer_bytes: 2097152      # 2MB
```

**Design considerations**:
- What happens when file is smaller than header+footer?
- Should priorities decay over time?
- How to handle multiple concurrent readers?

---

## Step 3.4: MP4 Moov Detection

Detect moov atom location for smarter prioritization.

**Context**:
- MP4 files have atoms: ftyp, moov, mdat (video data)
- moov at START = web-optimized, can stream immediately
- moov at END = must download footer first

**Approach**:
1. Read first 8 bytes at offset 0 → atom size + type
2. Scan atoms until moov found or limit reached
3. If not found at start, check end of file
4. Return moov location (offset, size) or nil

**Tasks**:
1. Create MP4Analyzer
2. ReadAtomHeader(offset) → name, size
3. FindMoov() → scan for moov, return location
4. Handle both start-of-file and end-of-file moov

**Edge cases**:
- Extended size atoms (size=1 means 64-bit size follows)
- File is not MP4 (return nil, not error)
- Very large moov atoms (rare but possible)

---

## Step 3.5: MKV Header Detection

Detect EBML/SeekHead for MKV files.

**Context**:
- MKV starts with EBML signature: 0x1A 0x45 0xDF 0xA3
- SeekHead near start indexes important elements
- Cues element contains seek points (may be near end)

**Simplified approach**:
- Verify EBML signature
- Assume first 10-20MB contains necessary metadata
- Return header size to prioritize

**Tasks**:
1. Create MKVAnalyzer
2. IsMKV() → check signature
3. FindSeekHead() → return byte offset to prioritize

**Note**: Full EBML parsing is complex. Start with conservative estimate (10MB), refine if needed.

---

## Step 3.6: PriorityReader Wrapper

Combine prioritization with reading.

**Context**:
- Wraps torrent file reader
- Updates priorities based on read operations
- Detects format and adjusts strategy

**Tasks**:
1. Create PriorityReader struct
2. Constructor: takes file, creates underlying reader, initializes prioritizer
3. Format detection: on first read, analyze MP4/MKV, adjust priorities
4. ReadAt(): update priorities for new position, then read
5. Read(): sequential read, update position tracking
6. Close(): cleanup

**Key behaviors**:
- Detect format once (cache result)
- On ReadAt (seek): call prioritizer.UpdateForSeek()
- Timeout on reads (don't block forever)
- Report activity for idle mode

---

## Step 3.7: Integrate with TorrentFile

Replace basic reader with PriorityReader.

**Tasks**:
1. Update TorrentFile.Open() to create PriorityReader
2. Ensure activity tracking still works
3. Test with various file types

**Flow**:
```
TorrentFile.Open()
  → torrentService.GetOrAddTorrent()
  → find file in torrent
  → create PriorityReader(file, torrent)
  → prioritizer.InitialPrioritize()
  → (async) format detection → adjust priorities
  → return wrapped reader
```

---

## Step 3.8: Reader Configuration

Tune anacrolix reader settings.

**Context**:
- anacrolix Reader has built-in readahead
- SetReadahead(bytes) - how far ahead to pre-download
- SetResponsive() - prioritize current read position

**Tasks**:
1. Configure reader readahead (16MB default)
2. Enable responsive mode
3. Test different settings

```go
reader := file.NewReader()
reader.SetReadahead(16 * 1024 * 1024)  // 16MB
reader.SetResponsive()  // prioritize current position
```

---

## Step 3.9: Timeout Handling

Don't hang on unavailable pieces.

**Context**:
- If pieces aren't available (no seeds), read blocks forever
- Need timeout to return error instead of blocking

**Tasks**:
1. Wrap reads with context timeout
2. Configure timeout (120s default)
3. Return appropriate error on timeout

**Implementation hint**:
```go
ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
defer cancel()
// Use reader.ReadContext(ctx, buf) if available
// Or wrap with goroutine + select
```

---

## Step 3.10: Testing & Tuning

Verify optimization works.

**Test scenarios**:

1. **Fast start**: Play video → should start within 5-10 seconds
2. **Seek forward**: Jump 50% into video → should resume within 5-10 seconds
3. **Seek backward**: Jump back → should work (pieces may be cached)
4. **MP4 end-moov**: Test file with moov at end → should still work

**Metrics to observe**:
- Time to first frame
- Time to resume after seek
- Piece download order (header/footer first?)
- Network utilization pattern

**Tuning parameters**:
- Header/footer priority bytes
- Readahead buffer size
- Urgent buffer size
- Read timeout

---

## Advanced Considerations (Future)

These are not required for Stage 3 but worth considering:

**Piece caching strategy**:
- Keep header pieces cached longer
- LRU eviction for body pieces
- Never evict currently-watching file

**Multiple reader handling**:
- Multiple clients streaming same file
- Shared prioritization vs independent

**Bandwidth estimation**:
- Detect available bandwidth
- Adjust readahead accordingly

**Quality-based selection**:
- If torrent has multiple quality versions
- Select based on bandwidth

---

## Suggested Additions After Stage 3

```
momoshtrem/
├── internal/
│   └── streaming/
│       ├── reader.go       # PriorityReader
│       ├── prioritizer.go  # Piece prioritization logic
│       ├── mp4.go          # MP4 moov detection
│       └── mkv.go          # MKV header detection
└── ...
```

---

## What Works After Stage 3

- ✅ Fast playback start (headers prioritized)
- ✅ Responsive seeking (position-based prioritization)
- ✅ Efficient bandwidth (readahead without waste)
- ✅ Format-aware optimization (MP4 moov, MKV headers)
- ✅ Timeout handling (no infinite blocks)

---

## Beyond Stage 3

Core streaming is now complete. Optional enhancements:

- **Stage 4**: Subtitles (OpenSubtitles integration)
- **Stage 5**: Skip Intro (EDL files for Infuse)
- **Stage 6**: Trakt integration

See main [README](./README.md) for overview.
