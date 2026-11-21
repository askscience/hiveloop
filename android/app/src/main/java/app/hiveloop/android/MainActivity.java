package app.hiveloop.android;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.app.DownloadManager;
import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Insets;
import android.os.Build;
import android.net.Uri;
import android.os.Bundle;
import android.os.Environment;
import android.view.View;
import android.view.WindowInsets;
import android.widget.FrameLayout;
import android.webkit.CookieManager;
import android.webkit.URLUtil;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Toast;

/**
 * Hiveloop for Android: a full-screen web view on the Hiveloop web app, nothing else.
 * Handles file uploads, downloads, the back button and external links.
 */
public class MainActivity extends Activity {
    private static final int FILE_REQUEST = 42;
    private WebView web;
    private ValueCallback<Uri[]> fileCallback;
    private String host;

    @SuppressLint("SetJavaScriptEnabled")
    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        host = Uri.parse(BuildConfig.HIVELOOP_URL).getHost();

        web = new WebView(this);
        web.setBackgroundColor(Color.parseColor("#0c0c0b"));
        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(Color.parseColor("#0c0c0b"));
        root.addView(web, new FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        setContentView(root);
        // Android 15+ draws apps edge-to-edge and ignores adjustResize: keep the page
        // between the status bar and the navigation bar / on-screen keyboard ourselves.
        if (Build.VERSION.SDK_INT >= 35) {
            root.setOnApplyWindowInsetsListener((v, insets) -> {
                Insets b = insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.ime() | WindowInsets.Type.displayCutout());
                v.setPadding(b.left, b.top, b.right, b.bottom);
                return WindowInsets.CONSUMED;
            });
        }

        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setDatabaseEnabled(true);
        s.setMediaPlaybackRequiresUserGesture(false);
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(true);
        s.setUserAgentString(s.getUserAgentString() + " HiveloopAndroid/" + BuildConfig.VERSION_NAME);
        s.setCacheMode(WebSettings.LOAD_NO_CACHE); // testing phase: always the latest interface

        CookieManager cookies = CookieManager.getInstance();
        cookies.setAcceptCookie(true);
        cookies.setAcceptThirdPartyCookies(web, false);

        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
                Uri u = req.getUrl();
                if (host != null && host.equals(u.getHost())) return false; // stay inside Hiveloop
                openExternal(u);
                return true;
            }

            @Override
            public void onReceivedError(WebView view, WebResourceRequest req, WebResourceError err) {
                if (req.isForMainFrame()) {
                    view.loadDataWithBaseURL(null, offlinePage(), "text/html", "utf-8", null);
                }
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                CookieManager.getInstance().flush();
            }
        });

        web.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> cb, FileChooserParams params) {
                if (fileCallback != null) fileCallback.onReceiveValue(null);
                fileCallback = cb;
                try {
                    Intent i = params.createIntent();
                    if (params.getMode() == FileChooserParams.MODE_OPEN_MULTIPLE) i.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
                    startActivityForResult(i, FILE_REQUEST);
                } catch (ActivityNotFoundException e) {
                    fileCallback = null;
                    return false;
                }
                return true;
            }
        });

        // Downloads (documents) go through the system download manager, with the session cookie.
        web.setDownloadListener((url, userAgent, contentDisposition, mimeType, length) -> {
            try {
                DownloadManager.Request r = new DownloadManager.Request(Uri.parse(url));
                String name = URLUtil.guessFileName(url, contentDisposition, mimeType);
                r.setMimeType(mimeType);
                r.addRequestHeader("Cookie", CookieManager.getInstance().getCookie(url));
                r.addRequestHeader("User-Agent", userAgent);
                r.setTitle(name);
                r.setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED);
                r.setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, name);
                ((DownloadManager) getSystemService(DOWNLOAD_SERVICE)).enqueue(r);
                Toast.makeText(this, name, Toast.LENGTH_SHORT).show();
            } catch (Exception e) {
                openExternal(Uri.parse(url));
            }
        });

        // notifications: periodic check, and ask once for permission (Android 13+)
        NotifyJob.channel(this);
        NotifyJob.schedule(this);
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission("android.permission.POST_NOTIFICATIONS") != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{"android.permission.POST_NOTIFICATIONS"}, 7);
        }

        String opened = ownUrl(getIntent());
        if (opened != null) web.loadUrl(opened);
        else if (state != null) web.restoreState(state);
        else web.loadUrl(BuildConfig.HIVELOOP_URL);
    }

    /** a notification opens a page of Hiveloop */
    private String ownUrl(Intent i) {
        Uri d = i == null ? null : i.getData();
        return d != null && host != null && host.equals(d.getHost()) && "https".equals(d.getScheme()) ? d.toString() : null;
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        String opened = ownUrl(intent);
        if (opened != null && web != null) web.loadUrl(opened);
    }

    private void openExternal(Uri u) {
        try {
            startActivity(new Intent(Intent.ACTION_VIEW, u));
        } catch (ActivityNotFoundException ignored) {
        }
    }

    private String offlinePage() {
        return "<!doctype html><meta name=viewport content='width=device-width,initial-scale=1'>"
            + "<body style='margin:0;min-height:100vh;display:grid;place-items:center;background:#0c0c0b;color:#f2f0e9;font-family:sans-serif;text-align:center'>"
            + "<div><p style='font:italic 56px Georgia,serif;margin:0'>hiveloop.</p>"
            + "<p>Nessuna connessione.<br><small style='opacity:.6'>No connection.</small></p>"
            + "<button onclick=\"location.href='" + BuildConfig.HIVELOOP_URL + "'\" style='margin-top:12px;padding:10px 22px;border-radius:99px;border:0;background:#f2f0e9;color:#0c0c0b;font-size:15px'>Riprova</button></div>";
    }

    @Override
    protected void onActivityResult(int req, int res, Intent data) {
        if (req == FILE_REQUEST && fileCallback != null) {
            fileCallback.onReceiveValue(WebChromeClient.FileChooserParams.parseResult(res, data));
            fileCallback = null;
            return;
        }
        super.onActivityResult(req, res, data);
    }

    @Override
    public void onBackPressed() {
        if (web.canGoBack()) web.goBack();
        else super.onBackPressed();
    }

    @Override
    protected void onSaveInstanceState(Bundle out) {
        super.onSaveInstanceState(out);
        web.saveState(out);
    }

    private long pausedAt = 0;

    @Override
    protected void onPause() {
        super.onPause();
        pausedAt = System.currentTimeMillis();
        CookieManager.getInstance().flush();
        NotifyJob.appVisible = false;
        // what was shown inside the app is not notified again later
        Thread t = new Thread(() -> { try { NotifyJob.check(getApplicationContext(), false); } catch (Exception ignored) { } });
        t.start();
    }

    @Override
    protected void onResume() {
        super.onResume();
        NotifyJob.appVisible = true;
        // back after more than 30 minutes: start fresh (latest interface and data)
        if (pausedAt > 0 && System.currentTimeMillis() - pausedAt > 30 * 60 * 1000) web.reload();
    }
}
