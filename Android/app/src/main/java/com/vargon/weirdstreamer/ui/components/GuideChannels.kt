package com.vargon.weirdstreamer.ui.components

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.basicMarquee
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.wrapContentWidth
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shadow
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import com.vargon.weirdstreamer.api.models.GuideNow
import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Date
import java.util.Locale

/*
 * The channels of their own (docs/GUIDE.md), drawn over the Guide's picture: the Guide channel's
 * promo cards, preview captions and listings roll, and the Weather channel's pages. Sizes follow
 * the picture's height (as the web's container units), so they fill full screen the same way.
 */

private val Gold = Color(0xFFFFD23F)
private val LightBlue = Color(0xFF9FC3FF)

/** Text sizes as hundredths of [height]. */
private class Scale(private val height: Dp, private val density: androidx.compose.ui.unit.Density) {
    fun sp(hundredths: Float): TextUnit = with(density) { (height * (hundredths / 100f)).toSp() }
    fun dp(hundredths: Float): Dp = height * (hundredths / 100f)
}

@Composable
private fun ScaledText(text: String, size: TextUnit, modifier: Modifier = Modifier, color: Color = Color.White,
                       weight: FontWeight = FontWeight.Bold, maxLines: Int = 1, align: TextAlign? = null,
                       mono: Boolean = false, shadow: Boolean = true) {
    androidx.compose.material3.Text(
        text, modifier = modifier, maxLines = maxLines, overflow = TextOverflow.Ellipsis, textAlign = align,
        style = TextStyle(color = color, fontSize = size, fontWeight = weight, lineHeight = size * 1.15f,
            fontFamily = if (mono) FontFamily.Monospace else FontFamily.SansSerif,
            shadow = if (shadow) Shadow(Color.Black.copy(alpha = 0.6f), androidx.compose.ui.geometry.Offset(2f, 2f), 1f) else null)
    )
}

/** When a program airs: "Tonight at 8:00 PM", "Tomorrow at 9:00 AM", "Thursday at 7:00 PM". */
fun airsWhen(ms: Long, now: Long): String {
    val at = Calendar.getInstance().apply { timeInMillis = ms }
    val today = Calendar.getInstance().apply { timeInMillis = now }
    fun day(c: Calendar) = c.get(Calendar.YEAR) * 400 + c.get(Calendar.DAY_OF_YEAR)
    val days = day(at) - day(today)
    val time = java.text.DateFormat.getTimeInstance(java.text.DateFormat.SHORT).format(Date(ms))
    return when (days) {
        0 -> (if (at.get(Calendar.HOUR_OF_DAY) >= 17) "Tonight" else "Today") + " at " + time
        1 -> "Tomorrow at $time"
        else -> SimpleDateFormat("EEEE", Locale.getDefault()).format(Date(ms)) + " at " + time
    }
}

/** The Guide channel's card for a program ahead, with music. */
@Composable
fun GuidePromo(model: GuideModel, now: GuideNow?) {
    val p = now?.program?.takeIf { it.kind != "channel" }
    val on = now?.airsOn
    val color = runCatching { Color(android.graphics.Color.parseColor((on ?: now?.channel)?.color ?: "#6A3A62")) }
        .getOrDefault(Color(0xFF6A3A62))
    BoxWithConstraints(Modifier.fillMaxSize().background(Brush.radialGradient(listOf(color, Color(0xFF0B0B1E))))) {
        val s = Scale(maxHeight, LocalDensity.current)
        Row(Modifier.fillMaxSize().padding(horizontal = s.dp(8f), vertical = s.dp(6f)),
            verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(s.dp(6f))) {
            p?.poster?.let {
                AsyncImage(model = model.posterUrl(it), contentDescription = null, contentScale = ContentScale.Fit,
                    modifier = Modifier.fillMaxHeight(0.9f).border(2.dp, Color.White))
            }
            Column(verticalArrangement = Arrangement.spacedBy(s.dp(1.5f))) {
                (on ?: now?.channel)?.let { c ->
                    AsyncImage(model = model.logoUrl(c, 512), contentDescription = null, modifier = Modifier.height(s.dp(22f)))
                }
                if (p != null && on != null && now.airs > 0) {
                    ScaledText("${airsWhen(now.airs, model.serverNow())} on ${on.number}".uppercase(), s.sp(7f), color = Gold)
                    ScaledText(p.title, s.sp(11f), weight = FontWeight.Black, maxLines = 2)
                    (p.episodeLabel ?: p.year?.toString())?.let { ScaledText(it, s.sp(6f)) }
                } else {
                    ScaledText("STAY TUNED", s.sp(7f), color = Gold)
                    ScaledText("The Guide Channel", s.sp(11f), weight = FontWeight.Black)
                }
                model.musicCredit()?.let { ScaledText(it, s.sp(4f), color = Color.White.copy(alpha = 0.85f), weight = FontWeight.Normal) }
            }
        }
    }
}

/** Over a preview: when and where it airs. */
@Composable
fun PreviewCaption(now: GuideNow, modifier: Modifier) {
    val p = now.program ?: return
    val on = now.airsOn ?: return
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val s = Scale(maxHeight, LocalDensity.current)
        Row(Modifier.align(Alignment.BottomStart).padding(start = s.dp(3f), bottom = s.dp(4f))
            .background(Color(0xBF000028))) {
            Box(Modifier.width(s.dp(1.4f)).height(s.dp(16f)).background(Gold))
            Column(Modifier.padding(horizontal = s.dp(2.5f), vertical = s.dp(1.5f))) {
                ScaledText("${airsWhen(now.airs, System.currentTimeMillis())} · ${on.number} ${on.name}".uppercase(), s.sp(5f), color = Gold)
                ScaledText(p.title + (p.episodeLabel?.let { " · $it" } ?: ""), s.sp(6.5f))
            }
        }
    }
}

/** The Guide channel's lower half: every channel, now and the next two half hours, rolling up. */
@Composable
fun GuideListings(model: GuideModel, modifier: Modifier) {
    val data = model.data ?: return
    val channels = data.channels.filter { it.kind != "guide" }
    if (channels.isEmpty()) return
    val first = model.serverNow() / GuideModel.HALF_HOUR * GuideModel.HALF_HOUR
    val slots = listOf(first, first + GuideModel.HALF_HOUR, first + 2 * GuideModel.HALF_HOUR)
    val weights = listOf(0.22f, 0.26f, 0.26f, 0.26f)
    val roll = rememberInfiniteTransition(label = "listings")
    val progress by roll.animateFloat(0f, 1f, infiniteRepeatable(tween(channels.size * 3000, easing = LinearEasing), RepeatMode.Restart),
        label = "roll")
    BoxWithConstraints(modifier.background(Brush.verticalGradient(listOf(Color(0xFF0A1A6A), Color(0xFF06104A))))) {
        // sized from the whole picture: this half is 42% of it
        val s = Scale(maxHeight / 0.42f, LocalDensity.current)
        val rowHeight = s.dp(9f)
        val boxHeight = maxHeight
        Column(Modifier.fillMaxSize()) {
            Row(Modifier.fillMaxWidth().background(Color(0xFFC8A000)).padding(vertical = s.dp(0.6f))) {
                ScaledText(SimpleDateFormat("EEE, MMM d", Locale.getDefault()).format(Date(first)), s.sp(3.2f),
                    Modifier.weight(weights[0]).padding(horizontal = s.dp(1.5f)), color = Color(0xFF1A1A1A), shadow = false)
                slots.forEachIndexed { i, t ->
                    ScaledText(guideTime(androidx.compose.ui.platform.LocalContext.current, t), s.sp(3.2f),
                        Modifier.weight(weights[i + 1]).padding(horizontal = s.dp(1.5f)), color = Color(0xFF1A1A1A), shadow = false)
                }
            }
            Box(Modifier.fillMaxSize().clipToBounds()) {
                // enough copies to fill the box the whole way round, so the roll loops without a jump or a gap
                val copies = maxOf(2, kotlin.math.ceil(boxHeight / (rowHeight * channels.size)).toInt() + 1)
                Column(Modifier.offset(y = -(rowHeight * channels.size) * progress)) {
                    repeat(copies) {
                        channels.forEach { c ->
                            Row(Modifier.fillMaxWidth().height(rowHeight)) {
                                Row(Modifier.weight(weights[0]).fillMaxHeight().background(Color.White.copy(alpha = 0.08f))
                                    .padding(horizontal = s.dp(1f)), verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(s.dp(1f))) {
                                    AsyncImage(model = model.logoUrl(c, 256), contentDescription = null,
                                        modifier = Modifier.height(s.dp(6f)).width(s.dp(14f)))
                                    ScaledText("${c.number} ${c.name}", s.sp(2.6f))
                                }
                                slots.forEachIndexed { i, t ->
                                    val l = c.listings?.firstOrNull { it.start <= t && t < it.end }
                                    val continued = l != null && l.start < t && l.info?.kind != "channel"
                                    Box(Modifier.weight(weights[i + 1]).fillMaxHeight().padding(horizontal = s.dp(1f)),
                                        contentAlignment = Alignment.CenterStart) {
                                        ScaledText(l?.info?.title ?: "—", s.sp(3f),
                                            color = if (continued) Color.White.copy(alpha = 0.6f) else Color.White)
                                    }
                                }
                            }
                            Box(Modifier.fillMaxWidth().height(1.dp).background(Color.White.copy(alpha = 0.15f)))
                        }
                    }
                }
            }
        }
    }
}

/** The Weather channel: its pages, turned every 10 seconds by the server's clock (the same on every screen). */
@Composable
fun WeatherScreen(model: GuideModel, modifier: Modifier) {
    LaunchedEffect(Unit) { model.loadWeather() }
    val w = model.weather
    val clock = model.clock.takeIf { it > 0 } ?: model.serverNow()
    val pages = buildList {
        if (w?.current != null) add("now")
        if (!w?.periods.isNullOrEmpty()) { add("today"); add("extended") }
        if (w?.radar == true) add("radar")
    }
    val page = if (pages.isEmpty()) "none" else pages[((clock / 10_000) % pages.size).toInt()]
    LaunchedEffect(clock / 60_000) { model.loadWeather() } // read again once it's 5 minutes old
    BoxWithConstraints(modifier.background(Brush.verticalGradient(listOf(Color(0xFF1D3F9A), Color(0xFF0A1F5C), Color(0xFF081640))))) {
        val s = Scale(maxHeight, LocalDensity.current)
        val screenWidth = maxWidth
        Column(Modifier.fillMaxSize()) {
            // the gold bar: the page, the place, the time
            Row(Modifier.fillMaxWidth().background(Brush.verticalGradient(listOf(Color(0xFFE6B800), Color(0xFFB38600))))
                .padding(horizontal = s.dp(4f), vertical = s.dp(2f)), verticalAlignment = Alignment.CenterVertically) {
                val title = mapOf("now" to "Current Conditions", "today" to "Local Forecast", "extended" to "Extended Forecast",
                    "radar" to "Local Radar")[page] ?: "Local Forecast"
                ScaledText(title.uppercase(), s.sp(6f), Modifier.weight(1f), color = Color(0xFF10204A), weight = FontWeight.Black, shadow = false)
                ScaledText(w?.place ?: "", s.sp(4f), Modifier.padding(end = s.dp(3f)), color = Color(0xFF10204A), shadow = false)
                ScaledText(java.text.DateFormat.getTimeInstance(java.text.DateFormat.MEDIUM).format(Date(clock)), s.sp(4f),
                    color = Color(0xFF10204A), mono = true, shadow = false)
            }
            Box(Modifier.weight(1f).fillMaxWidth().padding(horizontal = s.dp(4f), vertical = s.dp(3f)), contentAlignment = Alignment.Center) {
                when (page) {
                    "now" -> WeatherNowPage(model, w!!.current!!, s)
                    "today" -> Row(Modifier.fillMaxSize(), verticalAlignment = Alignment.CenterVertically) {
                        w!!.periods!!.take(3).forEach { p ->
                            Column(Modifier.weight(1f), horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(s.dp(1.2f))) {
                                ScaledText(p.name.uppercase(), s.sp(4.6f), color = Gold, weight = FontWeight.Black)
                                AsyncImage(model = model.weatherIconUrl(p.icon), contentDescription = null, modifier = Modifier.size(s.dp(14f)))
                                ScaledText("${if (p.daytime) "High" else "Low"} ${p.temp}°", s.sp(6f), weight = FontWeight.Black,
                                    color = if (p.daytime) Color.White else LightBlue)
                                ScaledText(p.forecast, s.sp(3.6f), maxLines = 2, align = TextAlign.Center)
                                p.precip?.takeIf { it > 0 }?.let { ScaledText("$it% chance", s.sp(3.2f), color = LightBlue) }
                            }
                        }
                    }
                    "extended" -> Row(Modifier.fillMaxSize(), verticalAlignment = Alignment.CenterVertically) {
                        val periods = w!!.periods!!
                        val days = periods.mapIndexedNotNull { i, p ->
                            if (!p.daytime) null else p to periods.getOrNull(i + 1)?.takeIf { !it.daytime }
                        }.drop(1).take(5)
                        days.forEach { (p, night) ->
                            Column(Modifier.weight(1f), horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(s.dp(1.2f))) {
                                ScaledText(p.name.take(3).uppercase(), s.sp(4.6f), color = Gold, weight = FontWeight.Black)
                                AsyncImage(model = model.weatherIconUrl(p.icon), contentDescription = null, modifier = Modifier.size(s.dp(14f)))
                                ScaledText(p.forecast, s.sp(3.4f), maxLines = 2, align = TextAlign.Center)
                                ScaledText("${p.temp}°" + (night?.let { " / ${it.temp}°" } ?: ""), s.sp(5f), weight = FontWeight.Black)
                            }
                        }
                    }
                    "radar" -> AsyncImage(model = model.weatherRadarUrl(), contentDescription = "Radar", contentScale = ContentScale.Fit,
                        modifier = Modifier.fillMaxHeight().border(2.dp, Color.White))
                    else -> ScaledText(w?.error ?: "The forecast isn't in yet.", s.sp(5f), maxLines = 3, align = TextAlign.Center)
                }
            }
            // warnings scroll along the bottom in red; the music's credit stays on screen
            val alerts = w?.alerts.orEmpty()
            val credit = model.musicCredit()
            if (alerts.isNotEmpty() && credit != null) {
                ScaledText(credit, s.sp(2.6f), Modifier.fillMaxWidth().padding(horizontal = s.dp(4f), vertical = s.dp(0.6f)),
                    color = Color(0xFFC9DCFF), weight = FontWeight.Normal, align = TextAlign.End)
            }
            Box(Modifier.fillMaxWidth().height(s.dp(7f)).background(if (alerts.isNotEmpty()) Color(0xFFB00020) else Color.Black.copy(alpha = 0.35f))
                .clipToBounds(), contentAlignment = Alignment.CenterStart) {
                if (alerts.isNotEmpty()) {
                    val text = alerts.joinToString("   ✦   ") { it.headline.ifBlank { it.event } }
                    Ticker(text, s.sp(4f), screenWidth)
                } else if (credit != null) {
                    ScaledText(credit, s.sp(3f), Modifier.padding(horizontal = s.dp(4f)), weight = FontWeight.Normal)
                }
            }
        }
    }
}

@Composable
private fun WeatherNowPage(model: GuideModel, c: com.vargon.weirdstreamer.api.models.WeatherNow, s: Scale) {
    Row(Modifier.fillMaxSize(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.SpaceEvenly) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(s.dp(3f))) {
                AsyncImage(model = model.weatherIconUrl(c.icon), contentDescription = null, modifier = Modifier.size(s.dp(30f)))
                ScaledText(c.temp?.let { "$it°" } ?: "—", s.sp(22f), weight = FontWeight.Black)
            }
            ScaledText(c.conditions, s.sp(5f))
        }
        val facts = listOfNotNull(
            c.humidity?.let { "Humidity" to "$it%" },
            c.dewpoint?.let { "Dewpoint" to "$it°" },
            c.wind?.let { "Wind" to if (it == 0) "Calm" else "${c.windDir.orEmpty()} $it mph".trim() },
            c.gust?.let { "Gusts" to "$it mph" },
            c.pressure?.let { "Pressure" to String.format(Locale.US, "%.2f in", it) },
            c.visibility?.let { "Visibility" to "$it mi" },
            c.feelsLike?.takeIf { it != c.temp }?.let { "Feels like" to "$it°" },
        )
        Column(verticalArrangement = Arrangement.spacedBy(s.dp(1.2f))) {
            facts.forEach { (label, value) ->
                Row {
                    ScaledText(label, s.sp(4.6f), Modifier.width(s.dp(30f)), color = LightBlue)
                    ScaledText(value, s.sp(4.6f))
                }
            }
        }
    }
}

/** Text scrolling right to left, over and over. */
@OptIn(androidx.compose.foundation.ExperimentalFoundationApi::class)
@Composable
private fun Ticker(text: String, size: TextUnit, width: Dp) {
    ScaledText(text + "          ", size, Modifier.fillMaxWidth().basicMarquee(iterations = Int.MAX_VALUE, initialDelayMillis = 0,
        repeatDelayMillis = 0, spacing = androidx.compose.foundation.MarqueeSpacing(width / 3), velocity = width / 12), shadow = false)
}
