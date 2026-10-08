// The way in. Sparse on purpose: two fields, one button, and one way across at
// the bottom — the same shape the web screen settled on.

import { Ionicons } from '@expo/vector-icons';
import { router } from 'expo-router';
import { useState } from 'react';
import { KeyboardAvoidingView, Platform, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Banner, Button, Field, Muted, Title } from '@/components/ui';
import { ApiError } from '@/lib/api';
import { useSession } from '@/lib/session';
import { color, space } from '@/lib/theme';

export default function SignIn() {
  const { signIn, signUp } = useSession();

  const [mode, setMode] = useState<'signin' | 'signup'>('signin');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [emailError, setEmailError] = useState<string | null>(null);
  const [passwordError, setPasswordError] = useState<string | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const signup = mode === 'signup';

  const submit = async () => {
    setEmailError(null);
    setPasswordError(null);
    setFailure(null);
    setBusy(true);

    try {
      if (signup) await signUp(email.trim(), password);
      else await signIn(email.trim(), password);

      router.replace('/');
    } catch (err) {
      if (!(err instanceof ApiError)) {
        // Anything that is not an answer from the server is a bug or a dead
        // network, and saying nothing is how that stays invisible.
        setFailure('Could not reach the server. Check that it is running and on the same network.');

        return;
      }

      // Where the message goes matters: a field problem belongs under the field.
      if (err.code === 'email_taken') setEmailError('This email is already registered.');
      else if (err.code === 'invalid_email') setEmailError('Enter a valid email address.');
      else if (err.code === 'weak_password') setPasswordError('Password must be at least 8 characters.');
      else if (err.code === 'invalid_credentials') {
        setPassword('');
        setFailure('Email or password is incorrect.');
      } else setFailure(err.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <SafeAreaView style={st.safe} edges={['top', 'bottom']}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={st.fill}>
        <ScrollView contentContainerStyle={st.scroll} keyboardShouldPersistTaps="handled">
          <View style={st.brand}>
            <View style={st.mark}>
              <Ionicons name="ticket" size={22} color={color.onPrimary} />
            </View>
            <Text style={st.brandName}>SeatHold</Text>
          </View>

          <Title>{signup ? 'Create your account' : 'Welcome back'}</Title>
          <Muted style={st.blurb}>
            {signup
              ? 'Your tickets and the seats you are holding stay in one place.'
              : 'Sign in to hold seats and see your tickets.'}
          </Muted>

          {failure ? <Banner kind="error" title="That did not work" text={failure} /> : null}

          <Field
            label="Email"
            value={email}
            onChangeText={setEmail}
            placeholder="you@example.com"
            keyboardType="email-address"
            autoComplete="email"
            error={emailError}
          />

          <Field
            label="Password"
            value={password}
            onChangeText={setPassword}
            secure
            autoComplete={signup ? 'new-password' : 'password'}
            error={passwordError}
          />

          {signup ? <Muted style={st.hint}>At least 8 characters.</Muted> : null}

          <Button title={signup ? 'Create account' : 'Sign in'} onPress={submit} busy={busy} />

          <View style={st.swap}>
            <Muted>{signup ? 'Already have an account? ' : 'New to SeatHold? '}</Muted>
            <Pressable onPress={() => setMode(signup ? 'signin' : 'signup')} hitSlop={8}>
              <Text style={st.link}>{signup ? 'Sign in' : 'Create an account'}</Text>
            </Pressable>
          </View>

          <Pressable onPress={() => router.replace('/')} style={st.browse} hitSlop={8}>
            <Text style={st.browseText}>Browse events without an account</Text>
          </Pressable>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const st = StyleSheet.create({
  safe: { flex: 1, backgroundColor: color.bg },
  fill: { flex: 1 },
  scroll: { padding: space.gutter, paddingTop: 8, flexGrow: 1, justifyContent: 'center' },

  brand: { flexDirection: 'row', alignItems: 'center', gap: 10, marginBottom: 36 },
  mark: {
    width: 40,
    height: 40,
    borderRadius: 10,
    backgroundColor: color.primary,
    alignItems: 'center',
    justifyContent: 'center',
  },
  brandName: { fontSize: 20, fontWeight: '700', color: color.text },

  blurb: { marginTop: 6, marginBottom: 28 },
  hint: { fontSize: 13, marginTop: -10, marginBottom: 18 },

  swap: { flexDirection: 'row', justifyContent: 'center', marginTop: 22, flexWrap: 'wrap' },
  link: { color: color.primary, fontWeight: '600', fontSize: 16, textDecorationLine: 'underline' },

  browse: { marginTop: 26, alignItems: 'center' },
  browseText: { color: color.textMuted, fontSize: 15, textDecorationLine: 'underline' },
});
