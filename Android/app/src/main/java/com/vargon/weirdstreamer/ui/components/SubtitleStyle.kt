package com.vargon.weirdstreamer.ui.components

import android.content.Context
import android.graphics.Typeface
import android.view.accessibility.CaptioningManager
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shadow
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

// The look of text subtitles on this device (Settings → Subtitle style): Android's own caption
// settings (Accessibility → Captions), or our own colours, outline, font and size.

enum class SubtitleBackground(val label: String) { BLACK("Black"), WHITE("White"), THEMED("Themed"), NONE("None") }
// NONE: see-through letters, cut out of the box or the outline, so the picture shows through them.
enum class SubtitleForeground(val label: String) { WHITE("White"), BLACK("Black"), THEMED("Themed"), NONE("None") }
enum class SubtitleFont(val label: String) { DEFAULT("Default"), SERIF("Serif"), MONOSPACE("Monospace"), CASUAL("Casual"), CONDENSED("Condensed") }
enum class SubtitleSize(val label: String, val scale: Float) {
    TINY("Tiny", 0.6f), SMALL("Small", 0.8f), NORMAL("Normal", 1f), LARGE("Large", 1.3f), HUGE("Huge", 1.6f)
}

/**
 * The phone player's Normal subtitle size: 18sp on a phone, growing with the screen's short side
 * past a phone's (about 411dp), a little less than in proportion: a tablet or an unfolded Fold
 * (750–800dp) gets about 1.6×, what Huge was there before.
 */
@Composable
fun phoneSubtitleFontSize(): TextUnit {
    val config = androidx.compose.ui.platform.LocalConfiguration.current
    val shortSide = minOf(config.screenWidthDp, config.screenHeightDp).toFloat()
    return 18.sp * Math.pow(shortSide / 411.0, 0.8).toFloat().coerceAtLeast(1f)
}

/** How see-through a background box is: 25% to solid. */
val SUBTITLE_OPACITIES = listOf(0.25f, 0.5f, 0.75f, 1f)
fun opacityLabel(o: Float) = if (o >= 1f) "Solid" else "${(o * 100).toInt()}%"

data class SubtitleStyleSetting(
    val system: Boolean = true, // Android's caption settings; the rest is the Custom style
    val background: SubtitleBackground = SubtitleBackground.BLACK,
    val opacity: Float = 0.5f,
    val foreground: SubtitleForeground = SubtitleForeground.WHITE,
    val textOpacity: Float = 1f, // the text's (and its outline's)
    val outline: Boolean = false, // the background colour outlines the text instead of a box
    val font: SubtitleFont = SubtitleFont.DEFAULT,
    val size: SubtitleSize = SubtitleSize.NORMAL,
)

/** The setting, kept on this device; players redraw when it changes. */
object SubtitleStyleStore {
    private const val PREFS = "subtitle_style"
    private var loaded = false
    var current by mutableStateOf(SubtitleStyleSetting())
        private set

    fun get(context: Context): SubtitleStyleSetting {
        if (!loaded) {
            loaded = true
            val p = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            val d = SubtitleStyleSetting()
            fun <E : Enum<E>> read(key: String, values: Array<E>, default: E) =
                values.firstOrNull { it.name == p.getString(key, null) } ?: default
            current = SubtitleStyleSetting(
                system = p.getBoolean("system", d.system),
                background = read("background", SubtitleBackground.entries.toTypedArray(), d.background),
                opacity = p.getFloat("opacity", d.opacity),
                foreground = read("foreground", SubtitleForeground.entries.toTypedArray(), d.foreground),
                textOpacity = p.getFloat("text_opacity", d.textOpacity),
                outline = p.getBoolean("outline", d.outline),
                font = read("font", SubtitleFont.entries.toTypedArray(), d.font),
                size = read("size", SubtitleSize.entries.toTypedArray(), d.size),
            )
        }
        return current
    }

    fun set(context: Context, s: SubtitleStyleSetting) {
        current = s
        loaded = true
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
            .putBoolean("system", s.system).putString("background", s.background.name).putFloat("opacity", s.opacity)
            .putString("foreground", s.foreground.name).putFloat("text_opacity", s.textOpacity).putBoolean("outline", s.outline)
            .putString("font", s.font.name).putString("size", s.size.name).apply()
    }
}

/** A style as drawn: text colour, a box behind it or an outline around it, a shadow, font and scale. */
data class ResolvedSubtitleStyle(
    val text: Color,
    val box: Color?,
    val outline: Color?,
    val shadow: Boolean,
    val font: FontFamily,
    val scale: Float,
    val textAlpha: Float = 1f, // the text and its outline, faded together
    val cutout: Boolean = false, // the letters erase the box or outline instead of being drawn
)

private fun fontFamily(font: SubtitleFont): FontFamily = when (font) {
    SubtitleFont.DEFAULT -> FontFamily.Default
    SubtitleFont.SERIF -> FontFamily.Serif
    SubtitleFont.MONOSPACE -> FontFamily.Monospace
    SubtitleFont.CASUAL -> FontFamily(Typeface.create("casual", Typeface.NORMAL))
    SubtitleFont.CONDENSED -> FontFamily(Typeface.create("sans-serif-condensed", Typeface.NORMAL))
}

/** The style to draw subtitles in now: the device's setting, with Android's captions or the theme resolved. */
@Composable
fun resolvedSubtitleStyle(setting: SubtitleStyleSetting = SubtitleStyleStore.get(LocalContext.current)): ResolvedSubtitleStyle {
    val context = LocalContext.current
    val spec = WeirdStreamerTheme.spec
    val accent = spec.focusColor.takeOrElse { Color(0xFFF6C445) }
    val panel = spec.panelColor.takeOrElse { Color(0xFF321B2F) }
    if (setting.system) {
        val cm = remember { context.getSystemService(Context.CAPTIONING_SERVICE) as? CaptioningManager }
        val cs = cm?.userStyle
        if (cm != null && cs != null) {
            val box = Color(cs.backgroundColor).takeIf { cs.hasBackgroundColor() && it.alpha > 0f }
            val edge = Color(cs.edgeColor).takeIf { cs.hasEdgeColor() } ?: Color.Black
            return ResolvedSubtitleStyle(
                text = if (cs.hasForegroundColor()) Color(cs.foregroundColor) else Color.White,
                box = if (cs.hasBackgroundColor()) box else Color.Black.copy(alpha = 0.55f),
                outline = edge.takeIf { cs.hasEdgeType() && cs.edgeType == CaptioningManager.CaptionStyle.EDGE_TYPE_OUTLINE },
                shadow = cs.hasEdgeType() && cs.edgeType == CaptioningManager.CaptionStyle.EDGE_TYPE_DROP_SHADOW,
                font = cs.typeface?.let { FontFamily(it) } ?: FontFamily.Default,
                scale = cm.fontScale,
            )
        }
    }
    val back = when (setting.background) {
        SubtitleBackground.BLACK -> Color.Black
        SubtitleBackground.WHITE -> Color.White
        SubtitleBackground.THEMED -> panel
        SubtitleBackground.NONE -> null
    }
    return ResolvedSubtitleStyle(
        text = when (setting.foreground) {
            SubtitleForeground.WHITE -> Color.White
            SubtitleForeground.BLACK -> Color.Black
            SubtitleForeground.THEMED -> accent
            SubtitleForeground.NONE -> Color.Black // only its shape is used, to cut with
        },
        box = back?.takeIf { !setting.outline }?.copy(alpha = setting.opacity),
        outline = back?.takeIf { setting.outline },
        shadow = false,
        font = fontFamily(setting.font),
        scale = setting.size.scale,
        textAlpha = setting.textOpacity,
        cutout = setting.foreground == SubtitleForeground.NONE,
    )
}

/** Subtitle text in a style: in its box, or with its outline drawn under the letters. */
@Composable
fun StyledSubtitleText(text: AnnotatedString, style: ResolvedSubtitleStyle, fontSize: TextUnit, modifier: Modifier = Modifier) {
    val size = fontSize * style.scale
    val base = TextStyle(color = style.text, fontSize = size, fontFamily = style.font, textAlign = TextAlign.Center,
        shadow = if (style.shadow) Shadow(Color.Black, blurRadius = 6f) else null)
    val boxed = if (style.box != null) Modifier.background(style.box, RoundedCornerShape(6.dp)).padding(horizontal = 10.dp, vertical = 4.dp)
        else Modifier.padding(horizontal = 10.dp, vertical = 4.dp)
    // A cut-out needs the box and outline in a layer of their own, for the letters to erase.
    val layer = if (style.cutout) Modifier.graphicsLayer { compositingStrategy = CompositingStrategy.Offscreen } else Modifier
    Box(modifier.then(layer).then(boxed), contentAlignment = Alignment.Center) {
        // The text and its outline fade as one (faded apart, the outline would show through the letters).
        Box(Modifier.graphicsLayer {
            alpha = style.textAlpha
            compositingStrategy = if (style.textAlpha < 1f) CompositingStrategy.Offscreen else CompositingStrategy.Auto
        }, contentAlignment = Alignment.Center) {
            style.outline?.let { outline ->
                val width = with(LocalDensity.current) { size.toPx() * 0.14f }
                androidx.compose.material3.Text(text, style = base.copy(color = outline, shadow = null,
                    drawStyle = Stroke(width = width, join = StrokeJoin.Round)))
            }
            if (!style.cutout) androidx.compose.material3.Text(text, style = base)
        }
        if (style.cutout) {
            androidx.compose.material3.Text(text, style = base.copy(shadow = null),
                modifier = Modifier.graphicsLayer { blendMode = BlendMode.DstOut })
        }
    }
}

/** What subtitles will look like: two lines over a picture-like backdrop. */
@Composable
fun SubtitleStylePreview(setting: SubtitleStyleSetting, fontSize: TextUnit, modifier: Modifier = Modifier) {
    Box(
        modifier.fillMaxWidth().height(170.dp)
            .background(Brush.linearGradient(listOf(Color(0xFF2B4A6B), Color(0xFF6B5A3A), Color(0xFFD8C48A))), RoundedCornerShape(10.dp))
            .padding(bottom = 16.dp),
        contentAlignment = Alignment.BottomCenter
    ) {
        StyledSubtitleText(AnnotatedString("The quick brown fox\njumps over the lazy dog."), resolvedSubtitleStyle(setting), fontSize)
    }
}
