// Constructs where ANTLR's error recovery depends on how its generated code
// matches: a set match followed by a token match (a failed set match leaves
// its error pending, and Match then returns nil and adds no conjured token),
// token and set labels after a set, a wildcard, and a block that lists every
// token type, which is a set match, not a wildcard.
grammar Recovery;

prog : item* EOF ;
item : (A | B) C D ';'
     | 'w' . D ';'
     | 'l' t=(A | B) u=C v=D ';'
     | 'all' (A | B | C | D | WS | ';' | 'w' | 'l' | 'all' | 'r') ';'
     | 'r' (A | B) r=rest ';'
     ;
rest : C D ;

A : 'a' ;
B : 'b' ;
C : 'c' ;
D : 'd' ;
WS : [ \t\r\n]+ -> skip ;
