// Decisions where ANTLR's SLL prediction picks an alternative its LL
// prediction does not: the SLL parse fails, and the parse ANTLR returns is
// the one LL prediction gives. galop's parsers must return the same one.
grammar Repair;

// SLL settles x's optional A by taking it, since global FOLLOW(x) allows
// what comes after; in s1's 'p' context that leaves no A for s1. The static
// lookahead automaton finds this conflict.
s1 : 'p' x A EOF | 'r' x EOF ;
x : A A? ;

// The same choice after nested parentheses: telling z's alternatives apart
// needs lookahead past n, which deepens the contexts without bound, so it is
// predicted at runtime.
s2 : 'p' z Q EOF | 'r' z EOF ;
z : n Q | n ;
n : '(' n ')' | B ;

// Several such conflicts in one input, through both kinds of prediction: the
// k items put ';' in FOLLOW(z), so a q item's z conflicts too.
s3 : ('p' x A ';' | 'r' x ';' | 'q' z Q ';' | 'k' z ';')+ EOF ;

// An entry rule without EOF, which galop's parsers do not repair: they parse
// again in LL mode, as ANTLR does.
s4 : 'p' x A | 'r' x ;

// SLL settles a's conflict with its first alternative, since global FOLLOW(a)
// has 'y'; in s5's 'c' context only the second can go on.
s5 : 'c' a 'z' EOF | a 'y' 'z' EOF ;
a : 'x' # A1 | 'x' 'y' # A2 ;

// Two such conflicts in one input.
s6 : 'c' a 'z' a 'z' EOF | a 'y' 'z' EOF ;

// An exact ambiguity: ANTLR takes the lower alternative, and nothing fails.
s7 : amb EOF ;
amb : 'x' # Amb1 | 'x' # Amb2 ;

// Two alternatives with the same future: LL takes the lower one, W2.
s8 : 'c' w 'z' EOF | w 'y' 'z' EOF ;
w : 'x' # W1 | 'x' 'y' # W2 | 'x' 'y' # W3 ;

// An entry rule without EOF that bracketed groups would mislead: over groups
// r9's first alternative could still end r9 inside u9, and a conflict at the
// end would take it; only the tokens inside the group rule that out. Such an
// entry predicts over tokens.
s9 : r9 ;
u9 : r9 '(' e9 ')' ;
r9 : 'x' | 'x' '(' f9 ')' ;
f9 : '(' f9 ')' | B ;
e9 : '(' e9 ')' | Q ;

// A greedy loop the first pass guesses without predicting: it goes on. In
// s10's 'c' context the loop must stop before its last item, which only the
// Q after that item shows (n nests deeper than static lookahead follows), so
// the guess fails and the search replays the parse with the loop stopping.
s10 : 'c' l10 'x' n Q EOF | 'r' l10 EOF ;
l10 : A ('x' n)* ;

// More such loops than the search may replay: the failed first pass is
// repaired instead.
s11 : ('c' l10 'x' n Q ';')+ EOF ;

// A prefix that can go on with the token that tells the alternatives apart:
// e12 can continue with '!', so a scan of e12 from the first token does not
// stop where stat12's e12 ends, and must not rule stat12's first alternative
// out by the token after it.
s12 : stat12+ EOF ;
stat12 : e12 '!' | e12 | '!' B ;
e12 : A ('!' B)? ;

A : 'a' ;
B : 'b' ;
Q : 'q' ;
WS : [ \t\r\n]+ -> skip ;
