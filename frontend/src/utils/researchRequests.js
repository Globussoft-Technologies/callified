export function createResearchRequests() {
  let sequence = 0;
  const current = new Map();
  return {
    begin(key) {
      const token = ++sequence;
      current.set(key, token);
      return token;
    },
    isCurrent(key, token) { return current.get(key) === token; },
    invalidate() { current.clear(); },
  };
}

export function finishResearchRequest(activeRequest, token, clearLoading) {
  if (activeRequest.current !== token) return;
  activeRequest.current = null;
  clearLoading();
}
