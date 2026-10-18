# Reverse proxy

Pocket Day speaks plain HTTP on one port and serves the UI and `/api/*` from the same binary. Set `BASE_URL` to the public URL and `SECURE_COOKIES=true` behind HTTPS. Keep `/api/*` reachable without forward-auth if phone shortcuts or a Homepage widget use bearer tokens.

## Caddy

```
day.example.com {
	reverse_proxy pocket-day:8080
}
```

## Traefik (labels)

```yaml
labels:
  - traefik.http.routers.day.rule=Host(`day.example.com`)
  - traefik.http.services.day.loadbalancer.server.port=8080
```

## nginx

```
location / {
    proxy_pass http://pocket-day:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 180s;   # a day refresh waits on every source
}
```

## SSO

`AUTH_MODE=proxy` trusts `Remote-User` / `Remote-Email` from Authelia, Authentik, Tinyauth or oauth2-proxy and creates accounts on first visit. Make sure the app is only reachable through the proxy.
