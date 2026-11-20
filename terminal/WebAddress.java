package dev.galaxy.terminal;

import java.io.UnsupportedEncodingException;
import java.net.URI;
import java.net.URISyntaxException;
import java.net.URLEncoder;

final class WebAddress {
    static String fromInput(String input) {
        String text = input.trim();
        if (text.isEmpty()) return null;
        boolean explicitScheme = text.matches("(?s)^[A-Za-z][A-Za-z0-9+.-]*://.*");
        URI address = parse(explicitScheme ? text : "https://" + text);
        if (explicitScheme) return allowed(address) ? address.toASCIIString() : null;

        String host = address == null ? null : address.getHost();
        if (host != null && (isLocal(host) || host.contains(".") || host.startsWith("[") || address.getPort() != -1)) {
            if (isLocal(host)) address = parse("http://" + text);
            return allowed(address) ? address.toASCIIString() : null;
        }
        if ((address != null && address.getRawUserInfo() != null) || text.matches("[A-Za-z][A-Za-z0-9+.-]*:\\S*")) {
            return null;
        }
        try {
            return "https://www.google.com/search?q=" + URLEncoder.encode(text, "UTF-8");
        } catch (UnsupportedEncodingException impossible) {
            throw new AssertionError(impossible);
        }
    }

    static boolean allowed(String url) { return url != null && allowed(parse(url)); }

    private static boolean allowed(URI address) {
        if (address == null || address.getHost() == null || address.getRawUserInfo() != null
                || address.getPort() == 0 || address.getPort() > 65535) return false;
        return "https".equalsIgnoreCase(address.getScheme())
                || ("http".equalsIgnoreCase(address.getScheme()) && isLocal(address.getHost()));
    }

    private static boolean isLocal(String host) {
        return "localhost".equalsIgnoreCase(host) || "127.0.0.1".equals(host);
    }

    private static URI parse(String address) {
        try { return new URI(address); }
        catch (URISyntaxException invalid) { return null; }
    }
}
