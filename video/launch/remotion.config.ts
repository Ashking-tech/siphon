import {existsSync} from 'node:fs';
import {Config} from '@remotion/cli/config';

// Local Chrome is available on this Linux host; the render command can still
// override this with --browser-executable if needed.
if (existsSync('/usr/bin/google-chrome')) {
  Config.setBrowserExecutable('/usr/bin/google-chrome');
}

Config.setVideoImageFormat('jpeg');
Config.setJpegQuality(94);
Config.setOverwriteOutput(true);
Config.setCodec('h264');
Config.setCrf(16);
Config.setPixelFormat('yuv420p');
// Standard HD tags and a limited-range conversion, rather than relying on
// ffmpeg's default full-range yuvj420p output.
Config.setColorSpace('bt709');
Config.setAudioCodec('aac');
Config.setAudioBitrate('320k');
Config.setConcurrency(4);
Config.setChromiumOpenGlRenderer('angle');
Config.setDelayRenderTimeoutInMilliseconds(120000);
