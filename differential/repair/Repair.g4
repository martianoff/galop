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

// Several such conflicts in one input.
s3 : ('p' x A ';' | 'r' x ';' | 'q' z Q ';')+ EOF ;

// An entry rule without EOF: ANTLR parses the longest prefix it can.
s4 : 'p' x A | 'r' x ;

A : 'a' ;
B : 'b' ;
Q : 'q' ;
WS : [ \t\r\n]+ -> skip ;
