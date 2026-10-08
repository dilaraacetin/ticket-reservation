// Who is signed in. One source, so no screen keeps its own copy of the answer.

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

import { ApiError, api, hasToken, loadToken, setToken } from './api';
import type { Account, Session } from './types';

type SessionValue = {
  account: Account | null;

  // False until the stored token has been tried, so the first frame does not
  // show a signed-out screen to somebody who is signed in.
  ready: boolean;

  signIn: (email: string, password: string) => Promise<void>;
  signUp: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
  refresh: () => Promise<void>;
};

const SessionContext = createContext<SessionValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [account, setAccount] = useState<Account | null>(null);
  const [ready, setReady] = useState(false);

  const refresh = useCallback(async () => {
    if (!hasToken()) {
      setAccount(null);

      return;
    }

    try {
      setAccount(await api<Account>('GET', '/account'));
    } catch (err) {
      // A token the server will not accept any more is not a token. Anything
      // else — the server being unreachable, say — leaves the session alone, so
      // a flaky network does not sign somebody out.
      if (err instanceof ApiError && err.status === 401) {
        await setToken(null);
        setAccount(null);
      }
    }
  }, []);

  useEffect(() => {
    (async () => {
      await loadToken();
      await refresh();
      setReady(true);
    })();
  }, [refresh]);

  const signIn = useCallback(
    async (email: string, password: string) => {
      const session = await api<Session>('POST', '/auth/login', { body: { email, password } });
      await setToken(session.token);
      await refresh();
    },
    [refresh],
  );

  const signUp = useCallback(
    async (email: string, password: string) => {
      await api('POST', '/auth/register', { body: { email, password } });
      await signIn(email, password);
    },
    [signIn],
  );

  const signOut = useCallback(async () => {
    try {
      await api('POST', '/auth/logout');
    } catch {
      // The token is being thrown away either way. Failing to tell the server
      // leaves it revocable server-side but useless here, which is the safe
      // direction for a sign-out.
    }

    await setToken(null);
    setAccount(null);
  }, []);

  const value = useMemo(
    () => ({ account, ready, signIn, signUp, signOut, refresh }),
    [account, ready, signIn, signUp, signOut, refresh],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionValue {
  const value = useContext(SessionContext);
  if (!value) throw new Error('useSession used outside SessionProvider');

  return value;
}
