import type {
  AuthResponse,
  LoginRequest,
  RegistrationRequest,
} from '../models/auth';
import type { User } from '../models/user';
import { authService } from './authService';
import { ApiError } from './http';
import {
  createSessionSync,
  type SessionEvent,
  type SessionSync,
} from './sessionSync';
import { getAccessToken, setAccessToken } from './tokenStorage';

export type SessionStatus = 'checking' | 'authenticated' | 'guest' | 'error';
export type SessionSnapshot = {
  status: SessionStatus;
  user: User | null;
  error: string | null;
  operation: 'login' | 'logout' | null;
  scope: number;
};
type RequestContext = { generation: number; revision: string | undefined };
type SessionTransport = Pick<
  typeof authService,
  'login' | 'refresh' | 'logout' | 'register'
>;

export class StaleSessionError extends Error {
  constructor() {
    super('Сессия изменилась. Повторите действие.');
    this.name = 'StaleSessionError';
  }
}

export class SessionCoordinator {
  private generation = 0;
  // Unlike request generations, this changes only when identity or user intent changes.
  private identityVersion = 0;
  private snapshot: SessionSnapshot = {
    status: 'checking',
    user: null,
    error: null,
    operation: null,
    scope: 0,
  };
  private listeners = new Set<() => void>();
  private refreshRequest: {
    generation: number;
    promise: Promise<void>;
  } | null = null;
  private logoutRequest: Promise<void> | null = null;
  private disconnect: (() => void) | null = null;
  private lastRevision: string | undefined;

  private transport: SessionTransport;
  private sync: SessionSync;

  constructor(transport: SessionTransport, sync: SessionSync) {
    this.transport = transport;
    this.sync = sync;
  }

  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  connect = () => {
    if (!this.disconnect) {
      this.lastRevision = this.sync.current()?.id;
      this.disconnect = this.sync.listen(this.receive);
    }
  };

  dispose = () => {
    this.disconnect?.();
    this.disconnect = null;
    this.identityVersion++;
    this.generation++;
    this.refreshRequest = null;
    this.logoutRequest = null;
    this.update(
      {
        status: 'checking',
        user: null,
        operation: null,
        error: null,
        scope: this.snapshot.scope + 1,
      },
      null,
    );
  };

  private update(snapshot: SessionSnapshot, token = getAccessToken()) {
    setAccessToken(token);
    this.snapshot = snapshot;
    this.listeners.forEach((listener) => listener());
  }

  private guest() {
    if (this.snapshot.user) this.identityVersion++;
    this.generation++;
    this.update(
      {
        status: 'guest',
        user: null,
        error: null,
        operation: null,
        scope: this.snapshot.scope + 1,
      },
      null,
    );
  }

  private accept(result: AuthResponse) {
    if (!result.user || !result.accessToken)
      throw new Error('Некорректный ответ авторизации');
    if (this.snapshot.user?.id !== result.user.id) this.identityVersion++;
    const scope =
      this.snapshot.scope +
      (this.snapshot.user && this.snapshot.user.id !== result.user.id ? 1 : 0);
    if (this.snapshot.user && this.snapshot.user.id !== result.user.id)
      this.generation++;
    this.update(
      {
        status: 'authenticated',
        user: result.user,
        error: null,
        operation: null,
        scope,
      },
      result.accessToken,
    );
  }

  private guard(generation: number) {
    if (generation !== this.generation) throw new StaleSessionError();
  }

  captureRequest = (): RequestContext => ({
    generation: this.generation,
    revision: this.sync.current()?.id,
  });

  assertRequest = (context: RequestContext) => {
    this.guard(context.generation);
    if (this.snapshot.operation || context.revision !== this.sync.current()?.id)
      throw new StaleSessionError();
  };

  rejectAccess = (context: RequestContext, token: string | null) => {
    this.assertRequest(context);
    if (token === getAccessToken()) this.guest();
  };

  initialize = (): Promise<void> => {
    this.connect();
    return this.snapshot.status === 'checking' && !this.snapshot.operation
      ? this.refresh()
      : Promise.resolve();
  };

  refresh = (): Promise<void> => {
    if (this.snapshot.operation) return Promise.reject(new StaleSessionError());
    const context = this.captureRequest();
    if (this.refreshRequest?.generation === context.generation)
      return this.refreshRequest.promise;
    const promise = this.sync
      .exclusive(async () => {
        this.assertRequest(context);
        try {
          const result = await this.transport.refresh();
          this.assertRequest(context);
          this.accept(result);
        } catch (error) {
          this.assertRequest(context);
          if (
            error instanceof ApiError &&
            (error.status === 401 ||
              (error.status === 403 && error.code === 'email_not_verified'))
          ) {
            this.guest();
          } else {
            this.update({
              ...this.snapshot,
              status: 'error',
              error:
                'Не удалось проверить сессию. Проверьте соединение и повторите попытку.',
            });
          }
          throw error;
        }
      })
      .finally(() => {
        if (this.refreshRequest?.promise === promise)
          this.refreshRequest = null;
      });
    this.refreshRequest = { generation: context.generation, promise };
    return promise;
  };

  login = (data: LoginRequest): Promise<void> => {
    this.identityVersion++;
    const generation = ++this.generation;
    this.update(
      {
        status: 'checking',
        user: null,
        error: null,
        operation: 'login',
        scope: this.snapshot.scope + 1,
      },
      null,
    );
    return this.sync.exclusive(async () => {
      this.guard(generation);
      try {
        const result = await this.transport.login(data);
        this.guard(generation);
        this.accept(result);
        this.lastRevision = this.sync.announce('login').id;
      } catch (error) {
        this.guard(generation);
        this.guest();
        throw error;
      }
    });
  };

  register = async (data: RegistrationRequest): Promise<void> => {
    const identity = this.identityVersion;
    const revision = this.sync.current()?.id;
    await this.transport.register(data);
    if (
      identity !== this.identityVersion ||
      revision !== this.sync.current()?.id
    )
      throw new StaleSessionError();
  };

  logout = (): Promise<void> => {
    if (this.snapshot.operation === 'logout' && this.logoutRequest)
      return this.logoutRequest;
    this.identityVersion++;
    const generation = ++this.generation;
    this.update({ ...this.snapshot, operation: 'logout', error: null });
    const promise = this.sync
      .exclusive(async () => {
        this.guard(generation);
        try {
          await this.transport.logout();
          this.guard(generation);
          this.guest();
          this.lastRevision = this.sync.announce('logout').id;
        } catch (error) {
          this.guard(generation);
          this.update({
            ...this.snapshot,
            status: this.snapshot.user ? 'authenticated' : 'error',
            operation: null,
            error: 'Выход не подтверждён сервером. Повторите попытку.',
          });
          throw error;
        }
      })
      .finally(() => {
        if (this.logoutRequest === promise) this.logoutRequest = null;
      });
    this.logoutRequest = promise;
    return promise;
  };

  private receive = (event: SessionEvent) => {
    if (event.id === this.lastRevision || this.snapshot.operation) return;
    this.identityVersion++;
    this.lastRevision = event.id;
    if (event.kind === 'logout') {
      this.guest();
      return;
    }
    this.generation++;
    this.update(
      {
        status: 'checking',
        user: null,
        error: null,
        operation: null,
        scope: this.snapshot.scope + 1,
      },
      null,
    );
    // Failures are visible in the snapshot; an event listener has no caller.
    void this.refresh().catch(() => {});
  };
}

export const sessionCoordinator = new SessionCoordinator(
  authService,
  createSessionSync(),
);
