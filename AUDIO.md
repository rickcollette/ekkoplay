# Mirrored audio outputs

`audio_outputs` defines one primary output and any number of mirrored secondary outputs. Each enabled output runs in its own supervised mpv process, so unplugging or losing a secondary USB device does not stop the primary output.

```json
"audio_outputs": [
  {
    "name": "Main room",
    "device": "alsa/default",
    "enabled": true,
    "primary": true,
    "volume_trim": 0,
    "muted": false,
    "delay_ms": 0,
    "buffer_ms": 100,
    "channels": "stereo",
    "sample_rate": 48000,
    "format": "float",
    "exclusive": false,
    "filter": "lavfi=[dynaudnorm=f=500:g=15:p=0.95]",
    "drift_correction_ms": 40
  },
  {
    "name": "USB DAC",
    "device": "alsa/plughw:CARD=USB,DEV=0",
    "enabled": true,
    "primary": false,
    "volume_trim": -8,
    "muted": false,
    "delay_ms": 35,
    "buffer_ms": 150,
    "channels": "stereo",
    "sample_rate": 48000,
    "format": "float",
    "exclusive": false,
    "filter": "",
    "drift_correction_ms": 40
  }
]
```

Use `mpv --audio-device=help` or Admin → Settings → Mirrored audio outputs to identify devices and monitor output health. The authenticated `GET /api/v1/admin/audio/devices` endpoint exposes the same discovery data.

Volume trim, mute, and delay compensation apply live and persist in SQLite. Device, enabled/primary state, channel layout, sample rate, sample format, exclusive mode, filter chain, buffer size, and drift threshold are startup settings; restart the player after changing them. Exactly one enabled output must be primary.

Separate USB sound cards have independent hardware clocks. `delay_ms` aligns their initial audible timing, while `drift_correction_ms` controls when a secondary is resynchronized to the primary transport. A lower threshold keeps rooms closer but can cause more corrections. Values around 30–60 ms are a practical starting point for separate rooms.
