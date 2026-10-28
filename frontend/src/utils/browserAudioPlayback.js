const SAMPLE_RATE = 8000;
const JITTER_TARGET_S = 0.08;
const MAX_BUFFER_S = 0.3;

// The JS queue excludes buffers already scheduled on the audio device. An
// empty queue therefore does not mean playback stopped. Preserve its deadline
// until the scheduled audio actually ends, or the next burst will overlap it.
export function drainPlaybackQueue(queue, state, now, schedule) {
  let { started, nextPlayTime } = state;
  if (queue.length === 0) {
    return { started: started && nextPlayTime > now, nextPlayTime };
  }

  let bufferedS = queue.reduce((sum, chunk) => sum + chunk.length, 0) / SAMPLE_RATE;
  while (bufferedS > MAX_BUFFER_S && queue.length > 0) {
    bufferedS -= queue.shift().length / SAMPLE_RATE;
  }

  // Rebuffer only after a real underrun, never while scheduled buffers remain.
  if (nextPlayTime <= now) started = false;
  if (!started) {
    if (bufferedS < JITTER_TARGET_S) return { started, nextPlayTime };
    started = true;
    nextPlayTime = Math.max(nextPlayTime, now + 0.02);
  }

  while (queue.length > 0 && nextPlayTime < now + MAX_BUFFER_S) {
    const chunk = queue.shift();
    const startAt = Math.max(nextPlayTime, now + 0.01);
    schedule(chunk, startAt);
    nextPlayTime = startAt + chunk.length / SAMPLE_RATE;
  }
  return { started, nextPlayTime };
}
