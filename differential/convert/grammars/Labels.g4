// Labels that can match more than once in a node, where ANTLR's label field
// holds only the last match: a token label used twice, labels in loops, the
// same label in two optional blocks and in two alternatives of a loop; next
// to list labels and set labels, and an ambiguous loop ANTLR resolves to its
// lower alternative.
grammar Labels;

prog : item* EOF ;
item : 'pair' a=ID ',' a=ID ';'               # Pair
     | 'seq' (x=ID)+ ';'                      # Seq
     | 'nest' (n=node ';')* 'end'             # Nest
     | 'opt' (o=ID)? (o=INT)? ';'             # Opt
     | 'mix' ('a' m=leaf | 'b' m=leaf)+ ';'   # Mix
     | 'list' ls+=node (',' ls+=node)* ';'    # List
     | 'set' s=(ID | INT) (',' s=(ID | INT))* ';' # Set
     | 'both' b=leaf b=leaf? ';'              # Both
     | 'amb' (y=ID | ID ID)* ';'              # Amb
     ;
node : '(' v=leaf (',' v=leaf)* ')' | leaf ;
leaf : ID | INT ;

ID : [a-z]+ ;
INT : [0-9]+ ;
WS : [ \t\r\n]+ -> skip ;
