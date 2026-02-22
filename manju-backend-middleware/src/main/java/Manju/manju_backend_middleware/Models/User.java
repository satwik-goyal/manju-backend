package Manju.manju_backend_middleware.Models;

import jakarta.persistence.*;
import lombok.Data;


@Entity
@Table(name = "ac_users")
@Data // Generates getters, setters, and toString via Lombok
public class User {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(unique = true, nullable = false)
    private String username;

    @Column(nullable = false)
    private String password;
}
