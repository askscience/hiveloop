package app.hiveloop.android;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.job.JobInfo;
import android.app.job.JobParameters;
import android.app.job.JobScheduler;
import android.app.job.JobService;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.Uri;
import android.os.Build;
import android.webkit.CookieManager;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;

/**
 * Checks Hiveloop for new notifications (private messages, mentions, volunteer
 * reminders) about every 15 minutes, with the app's own session, and shows them.
 * No Google services: Android runs the check when the network is available.
 */
public class NotifyJob extends JobService {
    private static final int JOB_ID = 1001;
    static final String CHANNEL = "hiveloop";
    static volatile boolean appVisible = false;

    /** schedule (or keep) the periodic check */
    static void schedule(Context c) {
        JobScheduler js = (JobScheduler) c.getSystemService(Context.JOB_SCHEDULER_SERVICE);
        if (js == null || js.getPendingJob(JOB_ID) != null) return;
        JobInfo job = new JobInfo.Builder(JOB_ID, new ComponentName(c, NotifyJob.class))
            .setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY)
            .setPeriodic(15 * 60 * 1000L)
            .setPersisted(true)
            .build();
        js.schedule(job);
    }

    @Override
    public boolean onStartJob(JobParameters params) {
        new Thread(() -> {
            try {
                check(getApplicationContext(), true);
            } catch (Exception ignored) {
            }
            jobFinished(params, false);
        }).start();
        return true;
    }

    @Override
    public boolean onStopJob(JobParameters params) {
        return true;
    }

    /** show = false only remembers the latest notice (the user has just seen the app). */
    static void check(Context c, boolean show) throws Exception {
        String base = BuildConfig.HIVELOOP_URL;
        String cookie = CookieManager.getInstance().getCookie(base);
        if (cookie == null || cookie.isEmpty()) return; // not signed in
        SharedPreferences prefs = c.getSharedPreferences("notify", Context.MODE_PRIVATE);
        long last = prefs.getLong("last", 0);
        HttpURLConnection conn = (HttpURLConnection) new URL(base + "api/notifications?after=" + last).openConnection();
        conn.setRequestProperty("Cookie", cookie);
        conn.setRequestProperty("Accept", "application/json");
        conn.setConnectTimeout(15000);
        conn.setReadTimeout(15000);
        if (conn.getResponseCode() != 200) return;
        JSONObject r = new JSONObject(read(conn.getInputStream()));
        long latest = r.optLong("latest", last);
        JSONArray items = r.optJSONArray("items");
        if (show && last > 0 && items != null && !appVisible) { // first check only remembers where we are
            for (int i = 0; i < items.length(); i++) show(c, items.getJSONObject(i));
        }
        if (latest != last) prefs.edit().putLong("last", latest).apply();
    }

    static void channel(Context c) {
        if (Build.VERSION.SDK_INT < 26) return;
        NotificationManager nm = c.getSystemService(NotificationManager.class);
        if (nm.getNotificationChannel(CHANNEL) == null) {
            nm.createNotificationChannel(new NotificationChannel(CHANNEL, "Hiveloop", NotificationManager.IMPORTANCE_HIGH));
        }
    }

    private static void show(Context c, JSONObject n) {
        channel(c);
        String url = n.optString("url", "/");
        Intent open = new Intent(c, MainActivity.class)
            .setAction(Intent.ACTION_VIEW)
            .setData(Uri.parse(BuildConfig.HIVELOOP_URL).buildUpon().encodedFragment(fragment(url)).build())
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        int id = (int) n.optLong("id", System.currentTimeMillis());
        PendingIntent pi = PendingIntent.getActivity(c, id, open, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        Notification.Builder b = Build.VERSION.SDK_INT >= 26 ? new Notification.Builder(c, CHANNEL) : new Notification.Builder(c);
        b.setSmallIcon(R.mipmap.ic_launcher)
            .setContentTitle(n.optString("title", "Hiveloop"))
            .setContentText(n.optString("body", ""))
            .setStyle(new Notification.BigTextStyle().bigText(n.optString("body", "")))
            .setAutoCancel(true)
            .setContentIntent(pi);
        String tag = n.optString("tag", "");
        NotificationManager nm = (NotificationManager) c.getSystemService(Context.NOTIFICATION_SERVICE);
        try {
            nm.notify(tag.isEmpty() ? null : tag, tag.isEmpty() ? id : 0, b.build());
        } catch (SecurityException ignored) { // notifications not allowed
        }
    }

    /** "/#/chat/-3" -> "/chat/-3" (the part after #) */
    private static String fragment(String url) {
        int i = url.indexOf('#');
        return i >= 0 ? url.substring(i + 1) : "/";
    }

    private static String read(InputStream in) throws Exception {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        byte[] buf = new byte[4096];
        int n;
        while ((n = in.read(buf)) > 0) {
            out.write(buf, 0, n);
            if (out.size() > 256 * 1024) break;
        }
        in.close();
        return out.toString("UTF-8");
    }
}
