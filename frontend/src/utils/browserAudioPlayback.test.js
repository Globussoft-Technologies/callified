import { test } from 'node:test';
import assert from 'node:assert/strict';
import { drainPlaybackQueue } from './browserAudioPlayback.js';

const frame = () => new Float32Array(160); // 20 ms at 8 kHz

test('a drained JS queue preserves scheduled audio and never overlaps the next burst', () => {
  const scheduled = [];
  const schedule = (pcm, start) => scheduled.push({ start, end: start + pcm.length / 8000 });
  let state = { started: false, nextPlayTime: 0 };
  state = drainPlaybackQueue(Array.from({ length: 4 }, frame), state, 0, schedule);
  assert.equal(scheduled.length, 4);
  state = drainPlaybackQueue([], state, 0.01, schedule);
  assert.equal(state.started, true);
  // Incoming packets arrive while the first burst is still on the device.
  state = drainPlaybackQueue([frame()], state, 0.02, schedule);
  assert.equal(scheduled.length, 5);
  for (let i = 1; i < scheduled.length; i++) {
    assert.ok(scheduled[i].start >= scheduled[i - 1].end, 'audio must not overlap');
  }
  assert.equal(state.nextPlayTime, scheduled.at(-1).end);
});

test('an actual underrun waits for the jitter buffer before resuming', () => {
  let state = { started: true, nextPlayTime: 0.1 };
  const queue = [frame()];
  const scheduled = [];
  state = drainPlaybackQueue(queue, state, 0.2, (_, start) => scheduled.push(start));
  assert.equal(state.started, false);
  assert.equal(scheduled.length, 0);
  queue.push(frame(), frame(), frame());
  state = drainPlaybackQueue(queue, state, 0.21, (_, start) => scheduled.push(start));
  assert.equal(state.started, true);
  assert.equal(scheduled.length, 4);
  assert.ok(scheduled[0] >= 0.23 - Number.EPSILON);
});

test('scheduled audio far ahead is never rewound over already queued buffers', () => {
  const scheduled = [];
  const queue = [frame()];
  const state = drainPlaybackQueue(queue, { started: true, nextPlayTime: 1 }, 0,
    (_, start) => scheduled.push(start));
  assert.equal(state.nextPlayTime, 1);
  assert.equal(queue.length, 1);
  assert.equal(scheduled.length, 0);
});
