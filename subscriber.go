package jessica

import (
	"encoding/binary"
	"net/http"
	"regexp"

	. "github.com/Monibuca/engine/v3"
	"github.com/Monibuca/utils/v3"
	"github.com/Monibuca/utils/v3/codec"
	"github.com/gobwas/ws"
)

var streamPathReg = regexp.MustCompile("/(jessica/)?((.+)(\\.flv)|(.+))")

func WsHandler(w http.ResponseWriter, r *http.Request) {
	isFlv := false
	parts := streamPathReg.FindStringSubmatch(r.RequestURI)
	if parts == nil {
		w.WriteHeader(404)
		return
	}
	streamPath := parts[3]
	if streamPath == "" {
		streamPath = parts[5]
	} else {
		isFlv = true
	}
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return
	}
	baseStream := Subscriber{ID: r.RemoteAddr, Type: "Jessica", Ctx2: r.Context()}
	if isFlv {
		baseStream.Type = "JessicaFlv"
	}
	defer conn.Close()
	go func() {
		b := []byte{0}
		for _, err := conn.Read(b); err == nil; _, err = conn.Read(b) {

		}
		baseStream.Close()
	}()
	if baseStream.Subscribe(streamPath) == nil {
		vt, at := baseStream.WaitVideoTrack(), baseStream.WaitAudioTrack()
		var writeAV func(byte, uint32, []byte)
		if isFlv {
			if err := ws.WriteHeader(conn, ws.Header{
				Fin:    true,
				OpCode: ws.OpBinary,
				Length: int64(13),
			}); err != nil {
				return
			}
			if _, err = conn.Write(codec.FLVHeader); err != nil {
				return
			}
			writeAV = func(t byte, ts uint32, payload []byte) {
				ws.WriteHeader(conn, ws.Header{
					Fin:    true,
					OpCode: ws.OpBinary,
					Length: int64(len(payload) + 15),
				})
				codec.WriteFLVTag(conn, t, ts, payload)
			}
		} else {
			writeAV = func(t byte, ts uint32, payload []byte) {
				ws.WriteHeader(conn, ws.Header{
					Fin:    true,
					OpCode: ws.OpBinary,
					Length: int64(len(payload) + 5),
				})
				head := utils.GetSlice(5)
				defer utils.RecycleSlice(head)
				head[0] = t - 7
				binary.BigEndian.PutUint32(head[1:5], ts)
				if _, err = conn.Write(head); err != nil {
					return
				}
				conn.Write(payload)
			}
		}
		if vt != nil {
			writeAV(codec.FLV_TAG_TYPE_VIDEO, 0, vt.ExtraData.Payload)
			baseStream.OnVideo = func(ts uint32, pack *VideoPack) {
				writeAV(codec.FLV_TAG_TYPE_VIDEO, ts, pack.Payload)
			}
		}
		if at != nil {
			writeAV(codec.FLV_TAG_TYPE_AUDIO, 0, at.ExtraData)
			baseStream.OnAudio = func(ts uint32, pack *AudioPack) {
				writeAV(codec.FLV_TAG_TYPE_AUDIO, ts, pack.Payload)
			}
		}
		baseStream.Play(at, vt)
	} else {
		w.WriteHeader(404)
	}
}
