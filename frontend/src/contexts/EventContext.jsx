import React, { createContext, useContext, useEffect, useRef, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useAuth } from './AuthContext';
import { API_URL } from '../constants/api';

const EventContext = createContext(null);

export function useCallifiedEvents() {
  return useContext(EventContext);
}

// Global window event name for listeners that don't want to use the React context.
export const CALLIFIED_EVENT_NAME = 'callified:event';

export function EventProvider({ children }) {
  const { fetchSseTicket, currentUser, authReady } = useAuth();
  const queryClient = useQueryClient();
  const esRef = useRef(null);
  const reconnectTimeoutRef = useRef(null);
  const listenersRef = useRef(new Set());

  const subscribe = useCallback((handler) => {
    listenersRef.current.add(handler);
    return () => listenersRef.current.delete(handler);
  }, []);

  const connect = useCallback(async () => {
    if (!authReady || !currentUser) return;
    if (esRef.current && esRef.current.readyState !== EventSource.CLOSED) return;

    try {
      const ticket = await fetchSseTicket();
      const es = new EventSource(`${API_URL}/events?ticket=${encodeURIComponent(ticket)}`);
      esRef.current = es;

      es.onopen = () => {
        // console.log('[events] connected');
      };

      es.onmessage = (ev) => {
        let payload;
        try {
          payload = JSON.parse(ev.data);
        } catch (e) {
          // Ignore malformed SSE payloads.
          return;
        }
        // Notify React subscribers.
        listenersRef.current.forEach((fn) => {
          try { fn(payload); } catch (_) {}
        });
        // Notify global window listeners (useful for class components and legacy hooks).
        window.dispatchEvent(new CustomEvent(CALLIFIED_EVENT_NAME, { detail: payload }));
      };

      es.onerror = () => {
        es.close();
        // Reconnect after a short delay with a fresh ticket.
        reconnectTimeoutRef.current = setTimeout(() => {
          connect();
        }, 3000);
      };
    } catch (err) {
      reconnectTimeoutRef.current = setTimeout(() => {
        connect();
      }, 5000);
    }
  }, [authReady, currentUser, fetchSseTicket]);

  useEffect(() => {
    connect();
    return () => {
      if (esRef.current) esRef.current.close();
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current);
    };
  }, [connect]);

  // Invalidate React Query caches when relevant domain events arrive.
  useEffect(() => {
    const handler = (ev) => {
      const payload = ev?.detail;
      if (!payload) return;
      const campaignId = payload.campaignId || payload.campaign_id;
      if (payload.type === 'LEAD_STATUS_CHANGED' && campaignId) {
        queryClient.invalidateQueries({ queryKey: ['campaign', campaignId, 'leads'] });
        queryClient.invalidateQueries({ queryKey: ['campaigns'] });
      }
      if (payload.type === 'CALL_COMPLETED' && campaignId) {
        queryClient.invalidateQueries({ queryKey: ['campaign', campaignId, 'leads'] });
        queryClient.invalidateQueries({ queryKey: ['campaign', campaignId, 'callLogs'] });
        queryClient.invalidateQueries({ queryKey: ['agentReport'] });
      }
      if (['CAMPAIGN_DIAL_STARTED', 'CAMPAIGN_DIAL_STOPPED'].includes(payload.type)) {
        queryClient.invalidateQueries({ queryKey: ['campaigns'] });
      }
    };
    window.addEventListener(CALLIFIED_EVENT_NAME, handler);
    return () => window.removeEventListener(CALLIFIED_EVENT_NAME, handler);
  }, [queryClient]);

  return (
    <EventContext.Provider value={{ subscribe }}>
      {children}
    </EventContext.Provider>
  );
}
