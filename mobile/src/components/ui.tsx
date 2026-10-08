// The pieces every screen is built from. One file, because there are few of them
// and keeping them together is what stops a second almost-identical button from
// being written somewhere else.

import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { useState, type ReactNode } from 'react';
import {
  ActivityIndicator,
  Image,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
  type StyleProp,
  type TextStyle,
  type ViewStyle,
} from 'react-native';

import { blend, color, radius, space, type } from '@/lib/theme';

export function Title({ children, style }: { children: ReactNode; style?: StyleProp<TextStyle> }) {
  return <Text style={[s.title, style]}>{children}</Text>;
}

export function Heading({ children, style }: { children: ReactNode; style?: StyleProp<TextStyle> }) {
  return <Text style={[s.heading, style]}>{children}</Text>;
}

export function Body({ children, style }: { children: ReactNode; style?: StyleProp<TextStyle> }) {
  return <Text style={[s.body, style]}>{children}</Text>;
}

export function Muted({ children, style }: { children: ReactNode; style?: StyleProp<TextStyle> }) {
  return <Text style={[s.muted, style]}>{children}</Text>;
}

export function Caption({ children, style }: { children: ReactNode; style?: StyleProp<TextStyle> }) {
  return <Text style={[s.caption, style]}>{children}</Text>;
}

export function Label({ children, style }: { children: ReactNode; style?: StyleProp<TextStyle> }) {
  return <Text style={[s.label, style]}>{children}</Text>;
}

type ButtonProps = {
  title: string;
  onPress: () => void;
  variant?: 'primary' | 'secondary' | 'danger';
  busy?: boolean;
  disabled?: boolean;
  icon?: keyof typeof Ionicons.glyphMap;
  style?: StyleProp<ViewStyle>;
};

export function Button({ title, onPress, variant = 'primary', busy, disabled, icon, style }: ButtonProps) {
  const off = disabled || busy;

  return (
    <Pressable
      accessibilityRole="button"
      disabled={off}
      onPress={onPress}
      style={({ pressed }) => [
        s.btn,
        variant === 'primary' && s.btnPrimary,
        variant === 'secondary' && s.btnSecondary,
        variant === 'danger' && s.btnDanger,
        off && s.btnOff,
        pressed && !off && s.btnPressed,
        style,
      ]}>
      {busy ? <ActivityIndicator color={variant === 'primary' ? color.onPrimary : color.text} /> : null}
      {!busy && icon ? (
        <Ionicons name={icon} size={20} color={variant === 'primary' ? color.onPrimary : color.text} />
      ) : null}
      <Text
        style={[
          s.btnText,
          variant === 'primary' && { color: color.onPrimary },
          variant === 'danger' && { color: color.danger },
          off && { color: color.textMuted },
        ]}>
        {title}
      </Text>
    </Pressable>
  );
}

type FieldProps = {
  label: string;
  value: string;
  onChangeText: (text: string) => void;
  placeholder?: string;
  secure?: boolean;
  error?: string | null;
  autoComplete?: 'email' | 'password' | 'new-password' | 'off';
  keyboardType?: 'default' | 'email-address';
};

export function Field({ label, value, onChangeText, placeholder, secure, error, autoComplete, keyboardType }: FieldProps) {
  const [hidden, setHidden] = useState(true);

  return (
    <View style={s.field}>
      <View style={s.fieldRow}>
        <Text style={s.fieldLabel}>{label}</Text>
        {secure ? (
          <Pressable onPress={() => setHidden((on) => !on)} hitSlop={10}>
            <Text style={s.reveal}>{hidden ? 'Show' : 'Hide'}</Text>
          </Pressable>
        ) : null}
      </View>

      <TextInput
        value={value}
        onChangeText={onChangeText}
        placeholder={placeholder}
        placeholderTextColor={color.textMuted}
        secureTextEntry={secure && hidden}
        autoCapitalize="none"
        autoCorrect={false}
        autoComplete={autoComplete}
        keyboardType={keyboardType}
        style={[s.input, error ? s.inputBad : null]}
      />

      {error ? (
        <View style={s.fieldError}>
          <Ionicons name="alert-circle" size={15} color={color.danger} />
          <Text style={s.fieldErrorText}>{error}</Text>
        </View>
      ) : null}
    </View>
  );
}

export function Banner({ kind, title, text }: { kind: 'error' | 'success' | 'warning' | 'info'; title: string; text?: string }) {
  const tone = {
    error: { bg: color.dangerBg, line: color.danger, icon: 'alert-circle' },
    success: { bg: color.successBg, line: color.success, icon: 'checkmark-circle' },
    warning: { bg: color.warningBg, line: color.warning, icon: 'warning' },
    info: { bg: color.infoBg, line: color.primary, icon: 'information-circle' },
  }[kind];

  return (
    <View style={[s.banner, { backgroundColor: tone.bg, borderColor: tone.line }]}>
      <Ionicons name={tone.icon as keyof typeof Ionicons.glyphMap} size={20} color={tone.line} />
      <View style={s.bannerBody}>
        <Text style={s.bannerTitle}>{title}</Text>
        {text ? <Text style={s.muted}>{text}</Text> : null}
      </View>
    </View>
  );
}

export function Tag({ text, tone = 'muted' }: { text: string; tone?: 'muted' | 'danger' | 'live' | 'onPoster' }) {
  const look = {
    muted: { bg: color.surfaceRaised, fg: color.textMuted },
    danger: { bg: blend.dangerSoft, fg: color.danger },
    live: { bg: blend.successSoft, fg: color.success },
    onPoster: { bg: blend.onPoster, fg: '#FFFFFF' },
  }[tone];

  return (
    <View style={[s.tag, { backgroundColor: look.bg }]}>
      <Text style={[s.tagText, { color: look.fg }]}>{text.toUpperCase()}</Text>
    </View>
  );
}

// A hue from the id, so an event with no poster still gets a face of its own and
// gets the same one every time. The same rule the web uses.
function hue(id: string): number {
  let total = 0;
  for (const ch of id) total = (total * 31 + ch.codePointAt(0)!) % 360;

  return total;
}

export function Poster({
  id,
  name,
  imageUrl,
  height,
  children,
}: {
  id: string;
  name: string;
  imageUrl?: string;
  height: number;
  children?: ReactNode;
}) {
  const [broken, setBroken] = useState(false);
  const h = hue(id);

  return (
    <View style={[s.poster, { height }]}>
      <LinearGradient
        colors={[`hsl(${h}, 42%, 40%)`, `hsl(${(h + 45) % 360}, 48%, 26%)`]}
        start={{ x: 0, y: 0 }}
        end={{ x: 1, y: 1 }}
        style={StyleSheet.absoluteFill}
      />

      {imageUrl && !broken ? (
        <Image
          source={{ uri: imageUrl }}
          style={StyleSheet.absoluteFill}
          resizeMode="cover"
          // A poster that will not load leaves the colour behind it rather than
          // an empty frame.
          onError={() => setBroken(true)}
        />
      ) : (
        <Text style={s.posterInitial}>{name.slice(0, 1).toUpperCase()}</Text>
      )}

      {children}
    </View>
  );
}

export function Empty({ icon, title, text, action }: { icon: keyof typeof Ionicons.glyphMap; title: string; text: string; action?: { label: string; onPress: () => void } }) {
  return (
    <View style={s.empty}>
      <Ionicons name={icon} size={64} color={color.textMuted} />
      <Heading style={{ marginTop: 14, textAlign: 'center' }}>{title}</Heading>
      <Muted style={{ textAlign: 'center', maxWidth: 300 }}>{text}</Muted>
      {action ? <Button title={action.label} onPress={action.onPress} style={{ marginTop: 14 }} /> : null}
    </View>
  );
}

export function Card({ children, style }: { children: ReactNode; style?: StyleProp<ViewStyle> }) {
  return <View style={[s.card, style]}>{children}</View>;
}

const s = StyleSheet.create({
  title: { ...type.title, color: color.text },
  heading: { ...type.heading, color: color.text },
  body: { ...type.body, color: color.text },
  muted: { ...type.body, color: color.textMuted },
  caption: { ...type.caption, color: color.textMuted },
  label: { ...type.label, color: color.primary },

  btn: {
    minHeight: space.tap,
    paddingHorizontal: 20,
    borderRadius: radius.base,
    borderWidth: 1,
    borderColor: 'transparent',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
  },
  btnPrimary: { backgroundColor: color.primary },
  btnSecondary: { backgroundColor: color.surfaceRaised, borderColor: color.lineStrong },
  btnDanger: { borderColor: blend.dangerSoft },
  btnOff: { backgroundColor: color.surfaceRaised, borderColor: color.line },
  btnPressed: { opacity: 0.85 },
  btnText: { fontSize: 16, fontWeight: '600', color: color.text },

  field: { gap: 8, marginBottom: 18 },
  fieldRow: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'baseline' },
  fieldLabel: { fontSize: 15, fontWeight: '600', color: color.text },
  reveal: { fontSize: 14, fontWeight: '600', color: color.textMuted },
  input: {
    minHeight: space.tap,
    paddingHorizontal: 16,
    borderRadius: radius.base,
    borderWidth: 1,
    borderColor: color.lineStrong,
    backgroundColor: color.surface,
    color: color.text,
    fontSize: 16,
  },
  inputBad: { borderColor: color.danger },
  fieldError: { flexDirection: 'row', alignItems: 'center', gap: 6 },
  fieldErrorText: { color: color.danger, fontSize: 13, flex: 1 },

  banner: {
    flexDirection: 'row',
    gap: 10,
    padding: 14,
    borderRadius: radius.base,
    borderWidth: 1,
    marginBottom: 14,
  },
  bannerBody: { flex: 1, gap: 2 },
  bannerTitle: { fontSize: 15, fontWeight: '700', color: color.text },

  tag: { paddingHorizontal: 10, paddingVertical: 4, borderRadius: radius.pill, alignSelf: 'flex-start' },
  tagText: { fontSize: 11, fontWeight: '700', letterSpacing: 0.9 },

  poster: { width: '100%', alignItems: 'center', justifyContent: 'center', overflow: 'hidden' },
  posterInitial: { fontSize: 56, fontWeight: '700', color: 'rgba(255,255,255,0.55)' },

  empty: { alignItems: 'center', justifyContent: 'center', paddingVertical: 48, gap: 6 },

  card: {
    backgroundColor: color.surface,
    borderWidth: 1,
    borderColor: color.line,
    borderRadius: radius.lg,
    padding: 16,
  },
});
