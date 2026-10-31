import {existsSync} from 'node:fs';
import {Config} from '@remotion/cli/config';

if (existsSync('/usr/bin/google-chrome')) {
  Config.setBrowserExecutable('/usr/bin/google-chrome');
}

Config.setVideoImageFormat('jpeg');
Config.setJpegQuality(95);
Config.setOverwriteOutput(true);
Config.setCodec('h264');
Config.setCrf(16);
Config.setPixelFormat('yuv420p');
Config.setColorSpace('bt709');
Config.setAudioCodec('aac');
Config.setAudioBitrate('320k');
Config.setConcurrency(4);
Config.setChromiumOpenGlRenderer('angle');
Config.setDelayRenderTimeoutInMilliseconds(120000);
