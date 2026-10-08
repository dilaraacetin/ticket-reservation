// The shapes the API answers with. Kept together so a screen never has to guess
// what a field is called, and so a change to the spec has one place to land.

export type Category = 'concert' | 'theatre' | 'comedy' | 'festival' | 'sport' | 'family' | 'other';

export type Event = {
  id: string;
  name: string;
  venue: string;
  startsAt: string;
  hasStarted: boolean;
  cancelled: boolean;
  cancelledAt?: string;

  city: string;
  category: Category;
  imageUrl: string;
  description: string;
  rules: string;
};

// What GET /events/{id} adds on top: the two numbers the detail screen leads on.
export type EventDetail = Event & {
  available: number;
  total: number;
};

export type SeatStatus = 'available' | 'held' | 'reserved';

export type Seat = {
  id: string;
  row: string;
  number: number;
  status: SeatStatus;
};

export type SeatMap = {
  eventId: string;
  seats: Seat[];
};

export type Hold = {
  holdId: string;
  eventId: string;
  seatId: string;
  userId: string;
  expiresAt: string;
  expiresInSeconds: number;
};

export type Account = {
  userId: string;
  email: string;
  role: 'customer' | 'admin';
  emailVerified: boolean;
};

export type Session = {
  token: string;
  userId: string;
  expiresAt: string;
  expiresInSeconds: number;
};

// One seat the caller has, held or confirmed. GET /tickets answers with both, so
// this is also how a hold is recovered after the app is reopened: the fields in
// the second group are present exactly while the seat is still only held.
export type Ticket = {
  eventId: string;
  eventName: string;
  venue: string;
  startsAt: string;
  seatId: string;
  row: string;
  number: number;
  status: SeatStatus;

  ticketCode?: string;

  holdId?: string;
  expiresAt?: string;
  expiresInSeconds?: number;
};

export type Tickets = {
  reservations: Ticket[];
};

export const CATEGORY_NAMES: Record<Category, string> = {
  concert: 'Concert',
  theatre: 'Theatre',
  comedy: 'Comedy',
  festival: 'Festival',
  sport: 'Sport',
  family: 'Family',
  other: 'Other',
};

// The order categories are offered in, mirroring domain.Categories. Other goes
// last: it is the fallback, not a choice to lead with.
export const CATEGORY_ORDER: Category[] = [
  'concert',
  'theatre',
  'comedy',
  'festival',
  'sport',
  'family',
  'other',
];
