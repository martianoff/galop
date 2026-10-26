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

A : 'a' ;
B : 'b' ;
Q : 'q' ;
WS : [ \t\r\n]+ -> skip ;
